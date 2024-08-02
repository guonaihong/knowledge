```go
package runtime

import (
	"internal/abi"
	"internal/cpu"
	"internal/goarch"
	"internal/goos"
	"internal/race"
	"internal/reflectlite"
	"internal/singleflight"
	"internal/syscall/windows"
	"internal/testlog"
	"internal/unsafeheader"
	"math"
	"reflect"
	"runtime/internal/atomic"
	"runtime/internal/math"
	"runtime/internal/sys"
	"sync/atomic"
	"unsafe"
)

// 设置使用cmd/go/internal/modload.ModInfoProg
var modinfo string

// Goroutine调度器
// 调度器的工作是将准备运行的goroutine分配到工作线程上。
//
// 主要概念包括：
// G - goroutine。
// M - 工作线程，或机器。
// P - 处理器，执行Go代码所需的资源。
//     M必须关联一个P才能执行Go代码，但它可以在没有关联P的情况下阻塞或在系统调用中。
//
// 设计文档位于 https://golang.org/s/go11sched。

// 工作线程的停车/启动。
// 我们需要在工作线程数量和停车过多的工作线程以节省CPU资源和电源之间取得平衡。这并不简单，原因如下：
// (1) 调度器状态是故意分散的（特别是每个P的工作队列），因此无法在快速路径上计算全局谓词；
// (2) 我们需要知道未来（不要在新的goroutine即将准备好时停车工作线程）。
//
// 三种被拒绝的方法：
// 1. 集中所有调度器状态（会抑制可扩展性）。
// 2. 直接goroutine交接。也就是说，当我们准备好一个新的goroutine并且有一个空闲的P时，启动一个线程并将其交给线程和goroutine。
//    这会导致线程状态抖动，因为准备goroutine的线程可以在下一刻没有工作，我们需要停车它。
//    此外，它会破坏计算的局部性，因为我们希望在同一个线程上保留相关的goroutine；并引入额外的延迟。
// 3. 当有一个空闲的P时，启动一个额外的线程，但不进行交接。这会导致过多的线程停车/启动，因为额外的线程会立即停车而没有发现任何工作要做。
//
// 当前的方法：
//
// 这种方法适用于三个主要的工作来源：准备好的goroutine、新的/更早修改的计时器和空闲优先级的GC。详情如下。
//
// 当我们提交工作时，如果（这是wakep()）：
// 1. 有一个空闲的P，并且
// 2. 没有“旋转”的工作线程。
//
// 旋转的工作线程是指没有本地工作并且没有在全局运行队列或netpoller中找到工作的线程；旋转状态在m.spinning和sched.nmspinning中表示。
// 这样的线程在停车之前会在P的运行队列和计时器堆中寻找工作，或者从GC中寻找工作。如果旋转的线程找到工作，它会退出旋转状态并继续执行。
// 如果它没有找到工作，它会退出旋转状态然后停车。
//
// 如果至少有一个旋转的线程（sched.nmspinning>1），我们在提交工作时不启动新的线程。为了补偿这一点，如果最后一个旋转的线程找到工作并停止旋转，它必须启动一个新的旋转线程。
// 这种方法平滑了不合理的线程启动峰值，但同时保证了最终的最大CPU并行度利用率。
//
// 主要的实现复杂性在于我们需要非常小心地在旋转->非旋转线程转换期间。这种转换可能与新工作的提交竞争，其中任何一个部分都需要启动另一个工作线程。
// 如果两者都失败了，我们可能会遇到半持久CPU利用不足的情况。
//
// 提交的一般模式是：
// 1. 将工作提交到本地或全局运行队列、计时器堆或GC状态。
// 2. #StoreLoad-style内存屏障。
// 3. 检查sched.nmspinning。
//
// 旋转->非旋转转换的一般模式是：
// 1. 减少nmspinning。
// 2. #StoreLoad-style内存屏障。
// 3. 检查所有P的工作队列和GC中的新工作。
//
// 注意，所有这些复杂性不适用于全局运行队列，因为我们不在提交到全局队列时草率地启动线程。另见nmspinning操作的注释。
//
// 这些不同工作来源的行为各不相同，但不会影响同步方法：
// * 准备好的goroutine：这是一个明显的工作来源；goroutine立即准备好并且最终必须在某个线程上运行。
// * 新的/更早修改的计时器：当前的计时器实现（见time.go）在一个没有可用工作的线程上使用netpoll等待最早的计时器。如果没有线程等待，我们希望一个新的旋转线程去等待。
// * 空闲优先级的GC：GC唤醒一个停止的空闲线程以贡献背景GC工作（注意：目前根据golang.org/issue/19112禁用）。另见golang.org/issue/44313，因为这应该扩展到所有GC工作线程。

var (
	m0           m
	g0           g
	mcache0      *mcache
	raceprocctx0 uintptr
	raceFiniLock mutex
)

// 这个切片记录了需要完成的初始化任务。它由链接器构建。
var runtime_inittasks []*initTask

// main_init_done是一个信号，用于cgocallbackg，表示初始化已完成。它在_cgo_notify_runtime_init_done之前完成，因此所有cgo调用都可以依赖它的存在。当main_init完成时，它被关闭，这意味着cgocallbackg可以可靠地从中接收。
var main_init_done chan bool

//go:linkname main_main main.main
func main_main()

// mainStarted表示主M已启动。
var mainStarted bool

// runtimeInitTime是运行时启动时的时间。
var runtimeInitTime int64

// 用于新创建的M的信号掩码的值。
var initSigmask sigset

// 主goroutine。
func main() {
	mp := getg().m

	// m0->g0的Racectx仅用作主goroutine的父级。它不能用于其他任何用途。
	mp.g0.racectx = 0

	// 最大栈大小在64位上是1 GB，在32位上是250 MB。
	// 使用十进制而不是二进制GB和MB，因为在栈溢出失败消息中看起来更好。
	if goarch.PtrSize == 8 {
		maxstacksize = 1000000000
	} else {
		maxstacksize = 250000000
	}

	// 最大栈大小的上限。用于避免在调用SetMaxStack并尝试分配太大的栈时随机崩溃，因为stackalloc使用32位大小。
	maxstackceiling = 2 * maxstacksize

	// 允许newproc启动新的Ms。
	mainStarted = true

	if haveSysmon {
		systemstack(func() {
			newm(sysmon, nil, -1)
		})
	}

	// 在初始化期间将主goroutine锁定到这个主OS线程上。大多数程序不会关心，但少数程序要求某些调用由主线程进行。可以在初始化期间调用runtime.LockOSThread来保留锁定。
	lockOSThread()

	if mp != &m0 {
		throw("runtime.main not on m0")
	}

	// 记录世界开始的时间。必须在doInit之前进行，以便进行初始化跟踪。
	runtimeInitTime = nanotime()
	if runtimeInitTime == 0 {
		throw("nanotime returning zero")
	}

	if debug.inittrace != 0 {
		inittrace.id = getg().goid
		inittrace.active = true
	}

	doInit(runtime_inittasks) // 必须在defer之前进行。

	// 延迟解锁，以便在初始化期间调用runtime.Goexit时也进行解锁。
	needUnlock := true
	defer func() {
		if needUnlock {
			unlockOSThread()
		}
	}()

	gcenable()

	main_init_done = make(chan bool)
	if iscgo {
		if _cgo_pthread_key_created == nil {
			throw("_cgo_pthread_key_created missing")
		}

		if _cgo_thread_start == nil {
			throw("_cgo_thread_start missing")
		}
		if GOOS != "windows" {
			if _cgo_setenv == nil {
				throw("_cgo_setenv missing")
			}
			if _cgo_unsetenv == nil {
				throw("_cgo_unsetenv missing")
			}
		}
		if _cgo_notify_runtime_init_done == nil {
			throw("_cgo_notify_runtime_init_done missing")
		}

		// 设置x_crosscall2_ptr C函数指针变量指向crosscall2。
		if set_crosscall2 == nil {
			throw("set_crosscall2 missing")
		}
		set_crosscall2()

		// 在从C创建的线程进入Go并需要创建新线程时启动模板线程。
		startTemplateThread()
		cgocall(_cgo_notify_runtime_init_done, nil)
	}

	// 运行初始化任务。根据构建模式，这个列表可以通过几种不同的方式到达，但它总是包含由链接器为程序中的所有包计算的初始化任务（不包括在运行时由包插件动态添加的任务）。按动态加载器初始化的顺序遍历模块（即它们被添加到moduledata链表中的顺序）。
	for m := &firstmoduledata; m != nil; m = m.next {
		doInit(m.inittasks)
	}

	// 在main init完成后禁用初始化跟踪，以避免malloc和newproc中的开销收集统计信息
	inittrace.active = false

	close(main_init_done)

	needUnlock = false
	unlockOSThread()

	if isarchive || islibrary {
		// 使用-buildmode=c-archive或c-shared编译的程序有一个main，但它不被执行。
		return
	}
	fn := main_main // 进行间接调用，因为链接器在布置运行时时不知道main包的地址
	fn()
	if raceenabled {
		runExitHooks(0) // 立即运行钩子，因为racefini不返回
		racefini()
	}

	// 使racy客户端程序工作：如果在main返回的同时在另一个goroutine上发生恐慌，让另一个goroutine完成打印恐慌跟踪。一旦它完成，它将退出。参见问题3934和20018。
	if runningPanicDefers.Load() != 0 {
		// 运行延迟函数不应花费很长时间。
		for c := 0; c < 1000; c++ {
			if runningPanicDefers.Load() == 0 {
				break
			}
			Gosched()
		}
	}
	if panicking.Load() != 0 {
		gopark(nil, nil, waitReasonPanicWait, traceBlockForever, 1)
	}
	runExitHooks(0)

	exit(0)
	for {
		var x *int32
		*x = 0
	}
}

// os_beforeExit 是从 os.Exit(0) 调用的。
//
//go:linkname os_beforeExit os.runtime_beforeExit
func os_beforeExit(exitCode int) {
	runExitHooks(exitCode)
	if exitCode == 0 && raceenabled {
		racefini()
	}
}

func init() {
	exithook.Gosched = Gosched
	exithook.Goid = func() uint64 { return getg().goid }
	exithook.Throw = throw
}

func runExitHooks(code int) {
	exithook.Run(code)
}

// 启动 forcegc 辅助 goroutine
func init() {
	go forcegchelper()
}

func forcegchelper() {
	forcegc.g = getg()
	lockInit(&forcegc.lock, lockRankForcegc)
	for {
		lock(&forcegc.lock)
		if forcegc.idle.Load() {
			throw("forcegc: phase error")
		}
		forcegc.idle.Store(true)
		goparkunlock(&forcegc.lock, waitReasonForceGCIdle, traceBlockSystemGoroutine, 1)
		// 这个 goroutine 被 sysmon 显式唤醒
		if debug.gctrace > 0 {
			println("GC forced")
		}
		// 时间触发的，完全并发。
		gcStart(gcTrigger{kind: gcTriggerTime, now: nanotime()})
	}
}

// Gosched 让出处理器，允许其他goroutine运行。它不会挂起当前goroutine，因此执行会自动恢复。
//
//go:nosplit
func Gosched() {
	checkTimeouts()
	mcall(gosched_m)
}

// goschedguarded 像 gosched 一样让出处理器，但也会检查禁止状态并在这些情况下选择不进行让出。
//
//go:nosplit
func goschedguarded() {
	mcall(goschedguarded_m)
}

// goschedIfBusy 像 gosched 一样让出处理器，但只有在没有空闲的Ps或我们在唯一的P上且运行队列中没有任何内容时才这样做。在这两种情况下，都有空闲时间可用。
//
//go:nosplit
func goschedIfBusy() {
	gp := getg()
	// 如果 gp.preempt 被设置，则调用 gosched；我们可能在不进行其他让出的情况下进入一个紧密循环。
	if !gp.preempt && sched.npidle.Load() > 0 {
		return
	}
	mcall(gosched_m)
}

// 将当前goroutine置于等待状态并在系统栈上调用unlockf。
// 如果unlockf返回false，则恢复goroutine。
// unlockf不能访问此G的栈，因为它可能在调用gopark和unlockf之间移动。
// 注意，因为unlockf是在将G置于等待状态后调用的，所以G可能在调用unlockf时已经被准备好，除非有外部同步防止G被准备好。如果unlockf返回false，它必须保证G不能被外部准备好。
//
// Reason解释了goroutine被停放的原因。它在栈跟踪和堆转储中显示。原因应该是唯一的和描述性的。不要重复使用原因，添加新的。
//
// gopark 应该是一个内部细节，
// 但广泛使用的包通过linkname访问它。
// 著名的“耻辱堂”成员包括：
//   - gvisor.dev/gvisor
//   - github.com/sagernet/gvisor
//
// 不要删除或改变类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname gopark
func gopark(unlockf func(*g, unsafe.Pointer) bool, lock unsafe.Pointer, reason waitReason, traceReason traceBlockReason, traceskip int) {
	if reason != waitReasonSleep {
		checkTimeouts() // 当两个goroutine保持调度器忙碌时，超时可能会过期
	}
	mp := acquirem()
	gp := mp.curg
	status := readgstatus(gp)
	if status != _Grunning && status != _Gscanrunning {
		throw("gopark: bad g status")
	}
	mp.waitlock = lock
	mp.waitunlockf = unlockf
	gp.waitreason = reason
	mp.waitTraceBlockReason = traceReason
	mp.waitTraceSkip = traceskip
	releasem(mp)
	// 在这里不能做任何可能移动G在Ms之间的事情。
	mcall(park_m)
}

// 将当前goroutine置于等待状态并解锁锁。
// 可以通过调用goready(gp)再次使goroutine可运行。
func goparkunlock(lock *mutex, reason waitReason, traceReason traceBlockReason, traceskip int) {
	gopark(parkunlock_c, unsafe.Pointer(lock), reason, traceReason, traceskip)
}

// goready 应该是一个内部细节，
// 但广泛使用的包通过linkname访问它。
// 著名的“耻辱堂”成员包括：
//   - gvisor.dev/gvisor
//   - github.com/sagernet/gvisor
//
// 不要删除或改变类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname goready
func goready(gp *g, traceskip int) {
	systemstack(func() {
		ready(gp, traceskip, true)
	})
}

//go:nosplit
func acquireSudog() *sudog {
    // 获取当前的 M（线程）
    mp := acquirem()
    // 获取当前的 P（处理器）：
    pp := mp.p.ptr()

    //如果本地缓存为空，则需要从中心缓存中获取 sudog。
    if len(pp.sudogcache) == 0 {
        lock(&sched.sudoglock)
        // 从中心缓存中获取 sudog：
        for len(pp.sudogcache) < cap(pp.sudogcache)/2 && sched.sudogcache != nil {
            s := sched.sudogcache
            sched.sudogcache = s.next
            s.next = nil
            pp.sudogcache = append(pp.sudogcache, s)
        }
        // 解锁中心缓存：
        unlock(&sched.sudoglock)
        // 如果本地缓存仍然为空，则创建一个新的 sudog：
        if len(pp.sudogcache) == 0 {
            pp.sudogcache = append(pp.sudogcache, new(sudog))
        }
    }

    // 从本地缓存中获取 sudog：
    n := len(pp.sudogcache)
    s := pp.sudogcache[n-1]
    pp.sudogcache[n-1] = nil
    // 从本地缓存中获取 sudog 并将其从缓存中移除。
    pp.sudogcache = pp.sudogcache[:n-1]
    if s.elem != nil {
        throw("acquireSudog: found s.elem != nil in cache")
    }
    // 释放 M：
    releasem(mp)
    // 返回 sudog：
    return s
}

// findRunnable 函数用于在 Go 调度器中查找一个可运行的 Goroutine。
// 返回值：gp 是找到的可运行 Goroutine，inheritTime 表示是否继承时间片，tryWakeP 表示是否尝试唤醒 P。
func findRunnable() (gp *g, inheritTime, tryWakeP bool) {
	mp := getg().m // 获取当前的 M（线程）

	// 如果调度器正在等待 GC，则停止当前 M 并重新开始查找。
top:
	pp := mp.p.ptr() // 获取当前的 P（处理器）
	if sched.gcwaiting.Load() {
		gcstopm()
		goto top
	}
	if pp.runSafePointFn != 0 {
		runSafePointFn()
	}

	// 保存当前时间和轮询时间，以便后续的工作窃取操作。
	now, pollUntil, _ := checkTimers(pp, 0)

	// 尝试调度 trace reader。
	if traceEnabled() || traceShuttingDown() {
		gp := traceReader()
		if gp != nil {
			trace := traceAcquire()
			casgstatus(gp, _Gwaiting, _Grunnable)
			if trace.ok() {
				trace.GoUnpark(gp, 0)
				traceRelease(trace)
			}
			return gp, false, true
		}
	}

	// 尝试调度一个 GC worker。
	if gcBlackenEnabled != 0 {
		gp, tnow := gcController.findRunnableGCWorker(pp, now)
		if gp != nil {
			return gp, false, true
		}
		now = tnow
	}

	// 偶尔检查全局可运行队列，以确保公平性。
	if pp.schedtick%61 == 0 && sched.runqsize > 0 {
		lock(&sched.lock)
		gp := globrunqget(pp, 1)
		unlock(&sched.lock)
		if gp != nil {
			return gp, false, false
		}
	}

	// 唤醒 finalizer Goroutine。
	if fingStatus.Load()&(fingWait|fingWake) == fingWait|fingWake {
		if gp := wakefing(); gp != nil {
			ready(gp, 0, true)
		}
	}
	if *cgo_yield != nil {
		asmcgocall(*cgo_yield, nil)
	}

	// 从本地运行队列获取 Goroutine。
	if gp, inheritTime := runqget(pp); gp != nil {
		return gp, inheritTime, false
	}

	// 从全局运行队列获取 Goroutine。
	if sched.runqsize != 0 {
		lock(&sched.lock)
		gp := globrunqget(pp, 0)
		unlock(&sched.lock)
		if gp != nil {
			return gp, false, false
		}
	}

	// 轮询网络。
	if netpollinited() && netpollAnyWaiters() && sched.lastpoll.Load() != 0 {
		if list, delta := netpoll(0); !list.empty() { // 非阻塞
			gp := list.pop()
			injectglist(&list)
			netpollAdjustWaiters(delta)
			trace := traceAcquire()
			casgstatus(gp, _Gwaiting, _Grunnable)
			if trace.ok() {
				trace.GoUnpark(gp, 0)
				traceRelease(trace)
			}
			return gp, false, false
		}
	}

	// 旋转的 M：从其他 P 窃取工作。
	if mp.spinning || 2*sched.nmspinning.Load() < gomaxprocs-sched.npidle.Load() {
		if !mp.spinning {
			mp.becomeSpinning()
		}

		gp, inheritTime, tnow, w, newWork := stealWork(now)
		if gp != nil {
			// 成功窃取。
			return gp, inheritTime, false
		}
		if newWork {
			// 可能有新的定时器或 GC 工作；重新开始查找。
			goto top
		}

		now = tnow
		if w != 0 && (pollUntil == 0 || w < pollUntil) {
			// 更早的定时器等待。
			pollUntil = w
		}
	}

	// 我们没有任何工作可做。
	if gcBlackenEnabled != 0 && gcMarkWorkAvailable(pp) && gcController.addIdleMarkWorker() {
		node := (*gcBgMarkWorkerNode)(gcBgMarkWorkerPool.pop())
		if node != nil {
			pp.gcMarkWorkerMode = gcMarkWorkerIdleMode
			gp := node.gp.ptr()

			trace := traceAcquire()
			casgstatus(gp, _Gwaiting, _Grunnable)
			if trace.ok() {
				trace.GoUnpark(gp, 0)
				traceRelease(trace)
			}
			return gp, false, false
		}
		gcController.removeIdleMarkWorker()
	}

	// wasm only:
	// 如果一个回调返回且没有其他 Goroutine 被唤醒，则唤醒事件处理 Goroutine。
	gp, otherReady := beforeIdle(now, pollUntil)
	if gp != nil {
		trace := traceAcquire()
		casgstatus(gp, _Gwaiting, _Grunnable)
		if trace.ok() {
			trace.GoUnpark(gp, 0)
			traceRelease(trace)
		}
		return gp, false, false
	}
	if otherReady {
		goto top
	}

	// 在我们释放 P 之前，获取 allp 切片的快照。
	allpSnapshot := allp
	idlepMaskSnapshot := idlepMask
	timerpMaskSnapshot := timerpMask

	// 返回 P 并阻塞。
	lock(&sched.lock)
	if sched.gcwaiting.Load() || pp.runSafePointFn != 0 {
		unlock(&sched.lock)
		goto top
	}
	if sched.runqsize != 0 {
		gp := globrunqget(pp, 0)
		unlock(&sched.lock)
		return gp, false, false
	}
	if !mp.spinning && sched.needspinning.Load() == 1 {
		mp.becomeSpinning()
		unlock(&sched.lock)
		goto top
	}
	if releasep() != pp {
		throw("findrunnable: wrong p")
	}
	now = pidleput(pp, now)
	unlock(&sched.lock)

	// 线程从旋转状态转换到非旋转状态，可能与提交新工作并发。
	wasSpinning := mp.spinning
	if mp.spinning {
		mp.spinning = false
		if sched.nmspinning.Add(-1) < 0 {
			throw("findrunnable: negative nmspinning")
		}

		// 检查全局和 P 运行队列。
		lock(&sched.lock)
		if sched.runqsize != 0 {
			pp, _ := pidlegetSpinning(0)
			if pp != nil {
				gp := globrunqget(pp, 0)
				if gp == nil {
					throw("global runq empty with non-zero runqsize")
				}
				unlock(&sched.lock)
				acquirep(pp)
				mp.becomeSpinning()
				return gp, false, false
			}
		}
		unlock(&sched.lock)

		pp := checkRunqsNoP(allpSnapshot, idlepMaskSnapshot)
		if pp != nil {
			acquirep(pp)
			mp.becomeSpinning()
			goto top
		}

		// 再次检查 idle-priority GC 工作。
		pp, gp := checkIdleGCNoP()
		if pp != nil {
			acquirep(pp)
			mp.becomeSpinning()

			// 运行 idle worker。
			pp.gcMarkWorkerMode = gcMarkWorkerIdleMode
			trace := traceAcquire()
			casgstatus(gp, _Gwaiting, _Grunnable)
			if trace.ok() {
				trace.GoUnpark(gp, 0)
				traceRelease(trace)
			}
			return gp, false, false
		}

		// 最后，检查定时器的创建或过期。
		pollUntil = checkTimersNoP(allpSnapshot, timerpMaskSnapshot, pollUntil)
	}

	// 轮询网络直到下一个定时器。
	if netpollinited() && (netpollAnyWaiters() || pollUntil != 0) && sched.lastpoll.Swap(0) != 0 {
		sched.pollUntil.Store(pollUntil)
		if mp.p != 0 {
			throw("findrunnable: netpoll with p")
		}
		if mp.spinning {
			throw("findrunnable: netpoll with spinning")
		}
		delay := int64(-1)
		if pollUntil != 0 {
			if now == 0 {
				now = nanotime()
			}
			delay = pollUntil - now
			if delay < 0 {
				delay = 0
			}
		}
		if faketime != 0 {
			delay = 0
		}
		list, delta := netpoll(delay) // 阻塞直到有新工作可用
		now = nanotime()
		sched.pollUntil.Store(0)
		sched.lastpoll.Store(now)
		if faketime != 0 && list.empty() {
			stopm()
			goto top
		}
		lock(&sched.lock)
		pp, _ := pidleget(now)
		unlock(&sched.lock)
		if pp == nil {
			injectglist(&list)
			netpollAdjustWaiters(delta)
		} else {
			acquirep(pp)
			if !list.empty() {
				gp := list.pop()
				injectglist(&list)
				netpollAdjustWaiters(delta)
				trace := traceAcquire()
				casgstatus(gp, _Gwaiting, _Grunnable)
				if trace.ok() {
					trace.GoUnpark(gp, 0)
					traceRelease(trace)
				}
				return gp, false, false
			}
			if wasSpinning {
				mp.becomeSpinning()
			}
			goto top
		}
	} else if pollUntil != 0 && netpollinited() {
		pollerPollUntil := sched.pollUntil.Load()
		if pollerPollUntil == 0 || pollerPollUntil > pollUntil {
			netpollBreak()
		}
	}
	stopm()
	goto top
}
```