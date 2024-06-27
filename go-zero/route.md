
## 使用例子

```
syntax = "v1"

type DemoPath3Req {
    Id int64 `path:"id"`
}

type DemoPath4Req {
    Id   int64  `path:"id"`
    Name string `path:"name"`
}

type DemoPath5Req {
    Id   int64  `path:"id"`
    Name string `path:"name"`
    Age  int    `path:"age"`
}

type DemoReq {}

type DemoResp {}

service Demo {
    // 示例路由 /foo
    @handler demoPath1
    get /foo (DemoReq) returns (DemoResp)

    // 示例路由 /foo/bar
    @handler demoPath2
    get /foo/bar (DemoReq) returns (DemoResp)

    // 示例路由 /foo/bar/:id，其中 id 为请求体中的字段
    @handler demoPath3
    get /foo/bar/:id (DemoPath3Req) returns (DemoResp)

    // 示例路由 /foo/bar/:id/:name，其中 id，name 为请求体中的字段
    @handler demoPath4
    get /foo/bar/:id/:name (DemoPath4Req) returns (DemoResp)

    // 示例路由 /foo/bar/:id/:name/:age，其中 id，name，age 为请求体中的字段
    @handler demoPath5
    get /foo/bar/:id/:name/:age (DemoPath5Req) returns (DemoResp)

    // 示例路由 /foo/bar/baz-qux
    @handler demoPath6
    get /foo/bar/baz-qux (DemoReq) returns (DemoResp)

    // 示例路由 /foo/bar_baz/123(goctl 1.5.1 支持)
    @handler demoPath7
    get /foo/bar_baz/123 (DemoReq) returns (DemoResp)
}


```
