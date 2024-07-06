### host 测试结果

```console
./fio.test: (g=0): rw=read, bs=(R) 4096B-4096B, (W) 4096B-4096B, (T) 4096B-4096B, ioengine=libaio, iodepth=64
fio-3.28
Starting 1 process
./fio.test: Laying out IO file (1 file / 10240MiB)
Jobs: 1 (f=1): [R(1)][100.0%][r=678MiB/s][r=174k IOPS][eta 00m:00s]
./fio.test: (groupid=0, jobs=1): err= 0: pid=41521: Sat Jul  6 18:08:55 2024
  read: IOPS=173k, BW=677MiB/s (710MB/s)(10.0GiB/15125msec)
    slat (nsec): min=2916, max=46359, avg=3540.94, stdev=2148.89
    clat (usec): min=130, max=8398, avg=365.36, stdev=66.83
     lat (usec): min=134, max=8406, avg=368.96, stdev=66.80
    clat percentiles (usec):
     |  1.00th=[  243],  5.00th=[  281], 10.00th=[  297], 20.00th=[  322],
     | 30.00th=[  334], 40.00th=[  351], 50.00th=[  367], 60.00th=[  379],
     | 70.00th=[  396], 80.00th=[  412], 90.00th=[  433], 95.00th=[  453],
     | 99.00th=[  490], 99.50th=[  506], 99.90th=[  570], 99.95th=[  619],
     | 99.99th=[  750]
   bw (  KiB/s): min=671832, max=695112, per=100.00%, avg=693563.73, stdev=4164.55, samples=30
   iops        : min=167958, max=173778, avg=173391.00, stdev=1041.16, samples=30
  lat (usec)   : 250=1.43%, 500=97.93%, 750=0.64%, 1000=0.01%
  lat (msec)   : 2=0.01%, 10=0.01%
  cpu          : usr=13.96%, sys=72.95%, ctx=204219, majf=0, minf=77
  IO depths    : 1=0.1%, 2=0.1%, 4=0.1%, 8=0.1%, 16=0.1%, 32=0.1%, >=64=100.0%
     submit    : 0=0.0%, 4=100.0%, 8=0.0%, 16=0.0%, 32=0.0%, 64=0.0%, >=64=0.0%
     complete  : 0=0.0%, 4=100.0%, 8=0.0%, 16=0.0%, 32=0.0%, 64=0.1%, >=64=0.0%
     issued rwts: total=2621440,0,0,0 short=0,0,0,0 dropped=0,0,0,0
     latency   : target=0, window=0, percentile=100.00%, depth=64

Run status group 0 (all jobs):
   READ: bw=677MiB/s (710MB/s), 677MiB/s-677MiB/s (710MB/s-710MB/s), io=10.0GiB (10.7GB), run=15125-15125msec

Disk stats (read/write):
  nvme0n1: ios=2597743/4, merge=0/1, ticks=892894/2, in_queue=892896, util=99.39%
```
