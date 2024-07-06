#/bin/bash
fio -direct=1 -iodepth=64 -rw=read -ioengine=libaio -bs=4k -size=10G -numjobs=1  -name=./fio.test

# -direct=1：使用直接 I/O（Direct I/O）模式，这意味着数据传输会绕过操作系统的缓存，直接在应用程序和存储设备之间进行。这通常用于模拟真实世界中的 I/O 操作，因为它避免了缓存的影响。
# -iodepth=64：设置 I/O 队列深度为 64。这意味着 fio 会同时提交 64 个 I/O 请求到存储设备。较大的队列深度可以提高异步 I/O 的效率，尤其是在使用支持大量并发 I/O 操作的存储设备时。
# -rw=read：指定测试类型为顺序读取（Sequential Read）。这意味着 fio 会从测试文件中顺序读取数据。
# -ioengine=libaio：指定 I/O 引擎为 libaio，即 Linux 异步 I/O。这允许 fio 使用异步 I/O 操作，从而提高 I/O 性能。
# -bs=4k：设置块大小（Block Size）为 4KB。这是每次 I/O 操作的数据大小。较小的块大小可以更好地模拟随机 I/O 操作。
# -size=10G：指定测试文件的大小为 10GB。fio 会创建一个 10GB 的文件来进行 I/O 测试。
# -numjobs=1：设置测试的并发任务数为 1。这意味着 fio 只会启动一个任务来执行 I/O 操作。
# -name=./fio.test：指定测试文件的名称为 ./fio.test。fio 会在当前目录下创建一个名为 fio.test 的文件来进行 I/O 测试。