package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Dispatch 按权限与审批策略调度工具调用。
// 先整批复检；任一待批则整批不执行并返回 PendingCalls。
// 通过后按串行或并行执行。业务失败只写入 Result，不返回 error。
// fail_fast 表示不再执行后续调用，未执行的补失败 Result。
func Dispatch(ctx context.Context, inv Invocation) (DispatchResult, error) {
	if inv.Registry == nil {
		return DispatchResult{}, fmt.Errorf("tool registry is required")
	}
	if inv.Mode == "" {
		inv.Mode = ExecutionSerial
	}
	if inv.FailurePolicy == "" {
		inv.FailurePolicy = FailureBestEffort
	}
	if inv.MaxParallel <= 0 {
		inv.MaxParallel = 1
	}

	approved := make(map[string]struct{}, len(inv.ApprovedCallIDs))
	for _, id := range inv.ApprovedCallIDs {
		approved[id] = struct{}{}
	}
	denied := make(map[string]struct{}, len(inv.DeniedCallIDs))
	for _, id := range inv.DeniedCallIDs {
		denied[id] = struct{}{}
	}

	prepared := make([]preparedCall, 0, len(inv.Calls))
	var approvalCalls []Call
	for i, call := range inv.Calls {
		if _, ok := denied[call.ID]; ok {
			prepared = append(prepared, preparedCall{
				call:     call,
				result:   failResult(call, "approval denied"),
				skip:     true,
				softFail: true,
			})
			continue
		}
		item, wait := prepareCall(inv, call, approved)
		if wait {
			approvalCalls = append(approvalCalls, call)
			continue
		}
		prepared = append(prepared, item)
		if inv.FailurePolicy == FailureFast && item.skip && !item.result.Success && !item.softFail {
			prepared = append(prepared, skippedRemaining(inv.Calls[i+1:])...)
			break
		}
	}
	if len(approvalCalls) > 0 {
		emit(inv, "approval_required", approvalCalls[0], max(1, approvalCalls[0].Attempt), nil)
		return DispatchResult{
			WaitingApproval: true,
			PendingCalls:    append([]Call(nil), inv.Calls...),
			ApprovalCalls:   approvalCalls,
		}, nil
	}

	if inv.Mode == ExecutionParallel && inv.MaxParallel > 1 {
		return runParallel(ctx, inv, prepared)
	}
	return runSerial(ctx, inv, prepared)
}

type preparedCall struct {
	call     Call
	tool     Tool
	result   Result
	skip     bool
	softFail bool
}

// prepareCall 查找工具并做参数、权限、审批校验；wait=true 表示需先审批。
// 查不到、参数错、权限不足都写成失败 Result，不返回 error。
func prepareCall(inv Invocation, call Call, approved map[string]struct{}) (preparedCall, bool) {
	emit(inv, "call_started", call, max(1, call.Attempt), nil)
	item, err := inv.Registry.Get(Reference{Name: call.Name})
	if err != nil {
		return preparedCall{call: call, result: failResult(call, err.Error()), skip: true}, false
	}
	def := item.Definition()
	if err := validateArguments(def, call.Arguments); err != nil {
		return preparedCall{call: call, result: failResult(call, err.Error()), skip: true}, false
	}
	if err := checkPermission(inv.PermissionPolicy, inv.AgentMode, def); err != nil {
		return preparedCall{call: call, result: failResult(call, err.Error()), skip: true}, false
	}
	if requiresApproval(inv.AgentMode, inv.ApprovalPolicy, def, call.ID, approved) {
		return preparedCall{}, true
	}
	return preparedCall{call: call, tool: item}, false
}

// runSerial 按调用顺序执行工具；fail_fast 遇失败 Result 后不再跑后续调用。
func runSerial(ctx context.Context, inv Invocation, items []preparedCall) (DispatchResult, error) {
	results := make([]Result, 0, len(items))
	for i, item := range items {
		if item.skip {
			results = append(results, item.result)
			if inv.FailurePolicy == FailureFast && !item.result.Success && !item.softFail {
				return DispatchResult{Results: appendSkippedResults(results, items[i+1:])}, nil
			}
			continue
		}
		result, err := executeOne(ctx, inv, item)
		results = append(results, result)
		if isInterrupt(err) {
			return DispatchResult{Results: appendSkippedResults(results, items[i+1:])}, err
		}
		if inv.FailurePolicy == FailureFast && !result.Success {
			return DispatchResult{Results: appendSkippedResults(results, items[i+1:])}, nil
		}
	}
	return DispatchResult{Results: results}, nil
}

// runParallel 在 MaxParallel 限制下并发执行。业务失败只写入 Result；取消/超时才返回 error。
func runParallel(ctx context.Context, inv Invocation, items []preparedCall) (DispatchResult, error) {
	results := make([]Result, len(items))
	group, ctx := withLimit(ctx, inv.MaxParallel)
	var interrupt error
	var mu sync.Mutex
	for i, item := range items {
		i, item := i, item
		group.go_(func() error {
			if item.skip {
				results[i] = item.result
				return nil
			}
			result, err := executeOne(ctx, inv, item)
			results[i] = result
			if isInterrupt(err) {
				mu.Lock()
				if interrupt == nil {
					interrupt = err
				}
				mu.Unlock()
				return err
			}
			return nil
		})
	}
	_ = group.wait()
	for i, item := range items {
		if results[i].CallID == "" && results[i].Name == "" && results[i].Error == "" {
			results[i] = failResult(item.call, "tool did not execute")
		}
	}
	return DispatchResult{Results: results}, interrupt
}

// executeOne 执行单个工具，按 SupportsRetry 做有限次退避重试。
// 业务失败只返回 Result{Success:false}；error 仅表示取消或超时。
func executeOne(ctx context.Context, inv Invocation, item preparedCall) (Result, error) {
	def := item.tool.Definition()
	attempt := max(1, item.call.Attempt)
	var last Result
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
		if isInterrupt(err) {
			if result.Error == "" {
				result.Error = err.Error()
			}
			result.Success = false
			emit(inv, "execution_result", item.call, attempt, &result)
			return result, err
		}
		if err != nil {
			result.Success = false
			if result.Error == "" {
				result.Error = err.Error()
			}
		}
		last = result
		if last.Success {
			emit(inv, "execution_result", item.call, attempt, &last)
			return last, nil
		}
		retryErr := fmt.Errorf("%s", last.Error)
		if last.Error == "" {
			retryErr = fmt.Errorf("tool failed")
		}
		if !def.SupportsRetry || !retryableTool(retryErr) || attempt >= maxAttempts(inv) {
			emit(inv, "execution_result", item.call, attempt, &last)
			return last, nil
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

// retryableTool 判断失败结果是否可重试；取消与超时不重试。
func retryableTool(err error) bool {
	if err == nil {
		return false
	}
	if isInterrupt(err) {
		return false
	}
	return true
}

func isInterrupt(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

func skippedRemaining(calls []Call) []preparedCall {
	out := make([]preparedCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, preparedCall{
			call:     call,
			result:   failResult(call, "tool did not execute"),
			skip:     true,
			softFail: true,
		})
	}
	return out
}

func appendSkippedResults(results []Result, rest []preparedCall) []Result {
	for _, item := range rest {
		if item.skip {
			results = append(results, item.result)
			continue
		}
		results = append(results, failResult(item.call, "tool did not execute"))
	}
	return results
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

// checkPermission 按 DeniedTools 与模式能力覆盖鉴权。
func checkPermission(policy PermissionPolicy, mode string, def Definition) error {
	for _, denied := range policy.DeniedTools {
		if denied == def.Name {
			return fmt.Errorf("%w: tool %q is denied", errPermissionDenied, def.Name)
		}
	}
	if !Covers(ModeCapabilities(mode), def.Permission.Capabilities) {
		return fmt.Errorf("%w: tool %q capabilities not covered by mode %s", errPermissionDenied, def.Name, mode)
	}
	return nil
}

// requiresApproval 判断该调用是否还要等人审；已批准、自动放行模式或工具声明无需审批则放行。
func requiresApproval(mode string, policy ApprovalPolicy, def Definition, callID string, approved map[string]struct{}) bool {
	if _, ok := approved[callID]; ok {
		return false
	}
	switch mode {
	case "auto_approve", "yolo", "ask", "plan":
		return false
	}
	for _, item := range policy.AutoApprovedTools {
		if item == def.Name {
			return false
		}
	}
	return def.Permission.RequiresApproval
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
	errPermissionDenied = fmt.Errorf("permission denied")
	errInvalidArguments = fmt.Errorf("invalid arguments")
)
