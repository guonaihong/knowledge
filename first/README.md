# 读写锁性能数据

```console
=== RUN   BenchmarkReadMore
BenchmarkReadMore
BenchmarkReadMore-8                  298           3981471 ns/op          113941 B/op       2020 allocs/op
=== RUN   BenchmarkReadMoreRW
BenchmarkReadMoreRW
BenchmarkReadMoreRW-8               1862            635531 ns/op          112335 B/op       2004 allocs/op
=== RUN   BenchmarkWriteMore
BenchmarkWriteMore
BenchmarkWriteMore-8                 304           3930633 ns/op          113980 B/op       2021 allocs/op
=== RUN   BenchmarkWriteMoreRW
BenchmarkWriteMoreRW
BenchmarkWriteMoreRW-8               325           3679236 ns/op          113764 B/op       2019 allocs/op
=== RUN   BenchmarkEqual
BenchmarkEqual
BenchmarkEqual-8                     300           3972177 ns/op          113796 B/op       2019 allocs/op
=== RUN   BenchmarkEqualRW
BenchmarkEqualRW
BenchmarkEqualRW-8                   561           2125922 ns/op          112945 B/op       2010 allocs/op
PASS
ok      github.com/guonaihong/question/first    9.115s
```
