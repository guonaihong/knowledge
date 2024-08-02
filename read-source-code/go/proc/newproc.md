```go
func runqput(pp *p, gp *g, next bool) {
    // 如果启用了随机化调度器，并且 next 为真，有 50% 的几率将 next 设置为 false。
    // 这用于引入一些调度的不确定性，防止调度的固定模式。
    if randomizeScheduler && next && randn(2) == 0 {
        next = false
    }

    // 如果 next 为真，将 gp 放入 pp.runnext 位置
    if next {
    retryNext:
        oldnext := pp.runnext
        // 尝试原子地将 gp 放入 pp.runnext，如果失败则重试
        if !pp.runnext.cas(oldnext, guintptr(unsafe.Pointer(gp))) {
            goto retryNext
        }
        if oldnext == 0 {
            return // 如果 pp.runnext 之前为空，直接返回
        }
        // 如果 pp.runnext 不为空，将旧的 runnext goroutine 放入常规运行队列
        gp = oldnext.ptr()
    }

retry:
    // 获取当前运行队列的头部索引
    h := atomic.LoadAcq(&pp.runqhead) // load-acquire，与消费者同步
    t := pp.runqtail // 获取当前运行队列的尾部索引
    // 如果队列未满，将 gp 放入尾部
    if t-h < uint32(len(pp.runq)) {
        pp.runq[t%uint32(len(pp.runq))].set(gp)
        // store-release，使该元素对消费者可用
        atomic.StoreRel(&pp.runqtail, t+1)
        return
    }
    // 如果运行队列已满，调用 runqputslow 处理
    if runqputslow(pp, gp, h, t) {
        return
    }
    // 运行队列没有满，确保上述放入操作成功
    goto retry
}

```