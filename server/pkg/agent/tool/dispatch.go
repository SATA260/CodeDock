package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Dispatch 按权限与审批策略调度工具调用。
// 先逐条校验/鉴权/判断审批；任一待批则立即返回 PendingCalls。
// 通过后按串行或并行执行，失败策略为 fail_fast / collect_all / best_effort。
func Dispatch(ctx context.Context, inv Invocation) (DispatchResult, error) {
	if inv.Registry == nil {
		return DispatchResult{}, fmt.Errorf("tool registry is required")
	}
	if inv.Mode == "" {
		inv.Mode = ExecutionSerial
	}
	if inv.FailurePolicy == "" {
		inv.FailurePolicy = FailureFast
	}
	if inv.MaxParallel <= 0 {
		inv.MaxParallel = 1
	}

	approved := make(map[string]struct{}, len(inv.ApprovedCallIDs))
	for _, id := range inv.ApprovedCallIDs {
		approved[id] = struct{}{}
	}

	prepared := make([]preparedCall, 0, len(inv.Calls))
	for _, call := range inv.Calls {
		item, result, wait, err := prepareCall(inv, call, approved)
		if err != nil {
			if inv.FailurePolicy == FailureFast {
				return DispatchResult{Results: []Result{result}}, err
			}
			prepared = append(prepared, preparedCall{call: call, result: result, skip: true})
			continue
		}
		if wait {
			emit(inv, "approval_required", call, call.Attempt, nil)
			return DispatchResult{
				Results:         collectPrepared(prepared),
				WaitingApproval: true,
				PendingCalls:    remainingCalls(inv.Calls, call),
			}, nil
		}
		prepared = append(prepared, item)
	}

	if inv.Mode == ExecutionParallel && inv.MaxParallel > 1 {
		return runParallel(ctx, inv, prepared)
	}
	return runSerial(ctx, inv, prepared)
}

type preparedCall struct {
	call   Call
	tool   Tool
	result Result
	skip   bool
}

// prepareCall 查找工具并做参数、权限、审批校验；wait=true 表示需先审批。
func prepareCall(inv Invocation, call Call, approved map[string]struct{}) (preparedCall, Result, bool, error) {
	emit(inv, "call_started", call, max(1, call.Attempt), nil)
	item, err := inv.Registry.Get(Reference{Name: call.Name})
	if err != nil {
		result := failResult(call, err.Error())
		return preparedCall{}, result, false, fmt.Errorf("%w: %s", errNonRetryable, err.Error())
	}
	def := item.Definition()
	if err := validateArguments(def, call.Arguments); err != nil {
		result := failResult(call, err.Error())
		return preparedCall{}, result, false, err
	}
	if err := checkPermission(inv.PermissionPolicy, inv.AgentMode, def); err != nil {
		result := failResult(call, err.Error())
		return preparedCall{}, result, false, err
	}
	if requiresApproval(inv.AgentMode, inv.ApprovalPolicy, def.Name, call.ID, approved) {
		return preparedCall{}, Result{}, true, nil
	}
	return preparedCall{call: call, tool: item}, Result{}, false, nil
}

// runSerial 按调用顺序执行工具；fail_fast 遇错即停。
func runSerial(ctx context.Context, inv Invocation, items []preparedCall) (DispatchResult, error) {
	results := make([]Result, 0, len(items))
	for _, item := range items {
		if item.skip {
			results = append(results, item.result)
			if inv.FailurePolicy == FailureFast && !item.result.Success {
				return DispatchResult{Results: results}, fmt.Errorf("%s", item.result.Error)
			}
			continue
		}
		result, err := executeOne(ctx, inv, item)
		results = append(results, result)
		if err != nil && inv.FailurePolicy == FailureFast {
			return DispatchResult{Results: results}, err
		}
	}
	return DispatchResult{Results: results}, nil
}

// runParallel 在 MaxParallel 限制下并发执行；fail_fast 会取消其余任务。
func runParallel(ctx context.Context, inv Invocation, items []preparedCall) (DispatchResult, error) {
	results := make([]Result, len(items))
	group, ctx := withLimit(ctx, inv.MaxParallel)
	var firstErr error
	var mu sync.Mutex
	for i, item := range items {
		i, item := i, item
		group.go_(func() error {
			if item.skip {
				results[i] = item.result
				if inv.FailurePolicy == FailureFast && !item.result.Success {
					return fmt.Errorf("%s", item.result.Error)
				}
				return nil
			}
			result, err := executeOne(ctx, inv, item)
			results[i] = result
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				if inv.FailurePolicy == FailureFast {
					return err
				}
			}
			return nil
		})
	}
	err := group.wait()
	if err != nil && inv.FailurePolicy == FailureFast {
		return DispatchResult{Results: results}, err
	}
	return DispatchResult{Results: results}, firstErr
}

// executeOne 执行单个工具，按 SupportsRetry 与可重试错误做有限次退避重试。
func executeOne(ctx context.Context, inv Invocation, item preparedCall) (Result, error) {
	def := item.tool.Definition()
	attempt := max(1, item.call.Attempt)
	var last Result
	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return failResult(item.call, err.Error()), err
		}
		emit(inv, "execution_started", item.call, attempt, nil)
		result, err := item.tool.Execute(ctx, Input{
			SessionID: inv.SessionID,
			RunID:     inv.RunID,
			TurnID:    inv.TurnID,
			Call:      item.call,
		})
		result.CallID = item.call.ID
		if result.Name == "" {
			result.Name = item.call.Name
		}
		last = result
		lastErr = err
		if err == nil && result.Success {
			emit(inv, "execution_result", item.call, attempt, &result)
			return result, nil
		}
		if err == nil && !result.Success {
			lastErr = fmt.Errorf("%s", result.Error)
		}
		if !def.SupportsRetry || !retryableTool(lastErr) || attempt >= maxAttempts(inv) {
			if last.Error == "" && lastErr != nil {
				last.Error = lastErr.Error()
			}
			last.Success = false
			emit(inv, "execution_result", item.call, attempt, &last)
			return last, lastErr
		}
		attempt++
		item.call.Attempt = attempt
		emit(inv, "execution_retry", item.call, attempt, nil)
		select {
		case <-ctx.Done():
			return failResult(item.call, ctx.Err().Error()), ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// maxAttempts 返回单次工具调用的最大尝试次数。
func maxAttempts(inv Invocation) int {
	return 3
}

// retryableTool 判断工具错误是否可重试；取消与超时不重试。
func retryableTool(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	return true
}

// validateArguments 检查参数是否为对象，并补齐 schema 声明的 required 字段。
func validateArguments(def Definition, raw json.RawMessage) error {
	if len(def.ParametersSchema) == 0 {
		return nil
	}
	var schema map[string]any
	if err := json.Unmarshal(def.ParametersSchema, &schema); err != nil {
		return nil
	}
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return fmt.Errorf("%w: %s", errInvalidArguments, err.Error())
	}
	required, _ := schema["required"].([]any)
	for _, item := range required {
		name, _ := item.(string)
		if name == "" {
			continue
		}
		if _, ok := args[name]; !ok {
			return fmt.Errorf("%w: missing %s", errInvalidArguments, name)
		}
	}
	return nil
}

// checkPermission 按 DeniedTools、AllowedCapabilities 与 ask/plan 禁写规则鉴权。
func checkPermission(policy PermissionPolicy, mode string, def Definition) error {
	for _, denied := range policy.DeniedTools {
		if denied == def.Name {
			return fmt.Errorf("%w: tool %q is denied", errPermissionDenied, def.Name)
		}
	}
	if len(policy.AllowedCapabilities) > 0 && len(def.Permission.Capabilities) > 0 {
		allowed := make(map[string]struct{}, len(policy.AllowedCapabilities))
		for _, cap := range policy.AllowedCapabilities {
			allowed[cap] = struct{}{}
		}
		ok := false
		for _, cap := range def.Permission.Capabilities {
			if _, exists := allowed[cap]; exists {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%w: tool %q capabilities not allowed", errPermissionDenied, def.Name)
		}
	}
	if (mode == "ask" || mode == "plan") && (def.Permission.Risk == RiskWrite || def.Permission.Risk == RiskAdmin) {
		return fmt.Errorf("%w: mode %s forbids %s tools", errPermissionDenied, mode, def.Permission.Risk)
	}
	return nil
}

// requiresApproval 判断该调用是否还要等人审；已批准或 auto/yolo/ask/plan 直接放行。
func requiresApproval(mode string, policy ApprovalPolicy, name, callID string, approved map[string]struct{}) bool {
	if _, ok := approved[callID]; ok {
		return false
	}
	switch mode {
	case "auto_approve", "yolo", "ask", "plan":
		return false
	}
	for _, item := range policy.AutoApprovedTools {
		if item == name {
			return false
		}
	}
	if len(policy.ApprovalRequiredFor) == 0 {
		return true
	}
	for _, item := range policy.ApprovalRequiredFor {
		if item == name {
			return true
		}
	}
	return false
}

// remainingCalls 返回从当前调用起（含自身）尚未执行的工具调用。
func remainingCalls(calls []Call, current Call) []Call {
	out := make([]Call, 0)
	seen := false
	for _, call := range calls {
		if call.ID == current.ID {
			seen = true
		}
		if seen {
			out = append(out, call)
		}
	}
	return out
}

// collectPrepared 收集准备阶段已失败并跳过的结果。
func collectPrepared(items []preparedCall) []Result {
	out := make([]Result, 0, len(items))
	for _, item := range items {
		if item.skip {
			out = append(out, item.result)
		}
	}
	return out
}

// failResult 构造一条失败的工具结果。
func failResult(call Call, message string) Result {
	return Result{CallID: call.ID, Name: call.Name, Success: false, Error: message}
}

// emit 把工具过程事件交给运行时钩子。
func emit(inv Invocation, kind string, call Call, attempt int, result *Result) {
	if inv.OnEvent == nil {
		return
	}
	inv.OnEvent(kind, call, attempt, result)
}

// max 返回两个整数中的较大值。
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	errNonRetryable     = fmt.Errorf("non-retryable")
	errPermissionDenied = fmt.Errorf("permission denied")
	errInvalidArguments = fmt.Errorf("invalid arguments")
)
