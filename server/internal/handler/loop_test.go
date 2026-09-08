package handler_test

import "testing"

// TestLoopRemoved 仅用于标记旧 Execute Loop 已被删除。
// 原 loop_test 依赖的长循环已不存在，骨架阶段不需要集成测试。
func TestLoopRemoved(t *testing.T) {
	t.Skip("旧 Execute Loop 已删除，三面骨架阶段不跑集成测试")
}
