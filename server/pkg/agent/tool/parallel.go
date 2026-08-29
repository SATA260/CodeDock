package tool

import (
	"context"
	"sync"
)

type limitedGroup struct {
	ctx     context.Context
	cancel  context.CancelFunc
	sem     chan struct{}
	wg      sync.WaitGroup
	errOnce sync.Once
	err     error
}

// withLimit 创建带并发上限的任务组；任一失败会取消其余任务。
func withLimit(ctx context.Context, n int) (*limitedGroup, context.Context) {
	if n <= 0 {
		n = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	return &limitedGroup{
		ctx:    ctx,
		cancel: cancel,
		sem:    make(chan struct{}, n),
	}, ctx
}

// go_ 领取一个信号量槽后启动任务。
func (g *limitedGroup) go_(fn func() error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		select {
		case <-g.ctx.Done():
			g.fail(g.ctx.Err())
			return
		case g.sem <- struct{}{}:
		}
		defer func() { <-g.sem }()
		if err := fn(); err != nil {
			g.fail(err)
		}
	}()
}

// fail 记录首个错误并取消组内其余任务。
func (g *limitedGroup) fail(err error) {
	g.errOnce.Do(func() {
		g.err = err
		g.cancel()
	})
}

// wait 等待全部任务结束并返回首个错误。
func (g *limitedGroup) wait() error {
	g.wg.Wait()
	g.cancel()
	return g.err
}
