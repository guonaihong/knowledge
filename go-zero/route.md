
### 路由支持的tag

* "path": url里面的变量
* "form": 查询字符串，或者表单是查询字符串的
* "header": http header
* "json": body里面带json的

### 使用例子

```go
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

### 核心代码分析

* 根据GET/POST/DELETE/PUT方法的不同, 保存到对应的search.Route里面
* children左边的节点表示匹配结束或者常量，右边的节点表示不匹配，需要继续匹配
* 如果实现变量捕获, forEach->match, 如果node保存的item是:开头的，直接把key和value保存到result里面

### 一、路由匹配

```go
package router

import (
 "errors"
 "net/http"
 "path"
 "strings"

 "github.com/zeromicro/go-zero/core/search"
 "github.com/zeromicro/go-zero/rest/httpx"
 "github.com/zeromicro/go-zero/rest/pathvar"
)

const (
 allowHeader          = "Allow"
 allowMethodSeparator = ", "
)

var (
 // ErrInvalidMethod is an error that indicates not a valid http method.
 ErrInvalidMethod = errors.New("not a valid http method")
 // ErrInvalidPath is an error that indicates path is not start with /.
 ErrInvalidPath = errors.New("path must begin with '/'")
)

type patRouter struct {
 trees      map[string]*search.Tree
 notFound   http.Handler
 notAllowed http.Handler
}

// NewRouter returns a httpx.Router.
func NewRouter() httpx.Router {
 return &patRouter{
  trees: make(map[string]*search.Tree),
 }
}

func (pr *patRouter) Handle(method, reqPath string, handler http.Handler) error {
 if !validMethod(method) {
  return ErrInvalidMethod
 }

 if len(reqPath) == 0 || reqPath[0] != '/' {
  return ErrInvalidPath
 }

 cleanPath := path.Clean(reqPath)
 tree, ok := pr.trees[method]
 if ok {
  return tree.Add(cleanPath, handler)
 }

 tree = search.NewTree()
 pr.trees[method] = tree
 return tree.Add(cleanPath, handler)
}

func (pr *patRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
 reqPath := path.Clean(r.URL.Path)
 if tree, ok := pr.trees[r.Method]; ok {
  if result, ok := tree.Search(reqPath); ok {
   if len(result.Params) > 0 {
    r = pathvar.WithVars(r, result.Params)
   }
   result.Item.(http.Handler).ServeHTTP(w, r)
   return
  }
 }

 allows, ok := pr.methodsAllowed(r.Method, reqPath)
 if !ok {
  pr.handleNotFound(w, r)
  return
 }

 if pr.notAllowed != nil {
  pr.notAllowed.ServeHTTP(w, r)
 } else {
  w.Header().Set(allowHeader, allows)
  w.WriteHeader(http.StatusMethodNotAllowed)
 }
}

func (pr *patRouter) SetNotFoundHandler(handler http.Handler) {
 pr.notFound = handler
}

func (pr *patRouter) SetNotAllowedHandler(handler http.Handler) {
 pr.notAllowed = handler
}

func (pr *patRouter) handleNotFound(w http.ResponseWriter, r *http.Request) {
 if pr.notFound != nil {
  pr.notFound.ServeHTTP(w, r)
 } else {
  http.NotFound(w, r)
 }
}

func (pr *patRouter) methodsAllowed(method, path string) (string, bool) {
 var allows []string

 for treeMethod, tree := range pr.trees {
  if treeMethod == method {
   continue
  }

  _, ok := tree.Search(path)
  if ok {
   allows = append(allows, treeMethod)
  }
 }

 if len(allows) > 0 {
  return strings.Join(allows, allowMethodSeparator), true
 }

 return "", false
}

func validMethod(method string) bool {
 return method == http.MethodDelete || method == http.MethodGet ||
  method == http.MethodHead || method == http.MethodOptions ||
  method == http.MethodPatch || method == http.MethodPost ||
  method == http.MethodPut
}

```

### 二、、给代码加上注释

```go
package search

import (
 "errors"
 "fmt"
)

const (
 colon = ':'
 slash = '/'
)

var (
 // errDupItem 表示添加重复的项。
 errDupItem = errors.New("duplicated item")
 // errDupSlash 表示项以多个斜杠开头。
 errDupSlash = errors.New("duplicated slash")
 // errEmptyItem 表示添加空项。
 errEmptyItem = errors.New("empty item")
 // errInvalidState 表示搜索树处于无效状态。
 errInvalidState = errors.New("search tree is in an invalid state")
 // errNotFromRoot 表示路径不是以斜杠开头。
 errNotFromRoot = errors.New("path should start with /")

 // NotFound 用于保存未找到的结果。
 NotFound Result
)

type (
 // innerResult 表示内部搜索结果。
 innerResult struct {
  key   string
  value string
  named bool
  found bool
 }

 // node 表示搜索树的节点。
 node struct {
  item     any
  children [2]map[string]*node
 }

 // Tree 表示一个搜索树。
 Tree struct {
  root *node
 }

 // Result 表示从树中搜索的结果。
 Result struct {
  Item   any
  Params map[string]string
 }
)

// NewTree 返回一个新的 Tree。
func NewTree() *Tree {
 return &Tree{
  root: newNode(nil),
 }
}

// Add 将项与路由关联。
func (t *Tree) Add(route string, item any) error {
 if len(route) == 0 || route[0] != slash {
  return errNotFromRoot
 }

 if item == nil {
  return errEmptyItem
 }

 err := add(t.root, route[1:], item)
 switch {
 case errors.Is(err, errDupItem):
  return duplicatedItem(route)
 case errors.Is(err, errDupSlash):
  return duplicatedSlash(route)
 default:
  return err
 }
}

// Search 搜索与给定路由关联的项。
func (t *Tree) Search(route string) (Result, bool) {
 if len(route) == 0 || route[0] != slash {
  return NotFound, false
 }

 var result Result
 ok := t.next(t.root, route[1:], &result)
 return result, ok
}

func (t *Tree) next(n *node, route string, result *Result) bool {
 if len(route) == 0 && n.item != nil {
  result.Item = n.item
  return true
 }

 for i := range route {
    // 找到/
  if route[i] != slash {
   continue
  }

  token := route[:i]
  return n.forEach(func(k string, v *node) bool {
   r := match(k, token)
   if !r.found || !t.next(v, route[i+1:], result) {
    return false
   }
   if r.named {
    addParam(result, r.key, r.value)
   }

   return true
  })
 }

 return n.forEach(func(k string, v *node) bool {
  if r := match(k, route); r.found && v.item != nil {
   result.Item = v.item
   if r.named {
    addParam(result, r.key, r.value)
   }

   return true
  }

  return false
 })
}

func (nd *node) forEach(fn func(string, *node) bool) bool {
 for _, children := range nd.children {
  for k, v := range children {
   if fn(k, v) {
    return true
   }
  }
 }

 return false
}

func (nd *node) getChildren(route string) map[string]*node {
 if len(route) > 0 && route[0] == colon {
  return nd.children[1]
 }

 return nd.children[0]
}

func add(nd *node, route string, item any) error {
 if len(route) == 0 {
  if nd.item != nil {
   return errDupItem
  }

  nd.item = item
  return nil
 }

 if route[0] == slash {
  return errDupSlash
 }

 for i := range route {
  if route[i] != slash {
   continue
  }

  token := route[:i]
  children := nd.getChildren(token)
  if child, ok := children[token]; ok {
   if child == nil {
    return errInvalidState
   }

   return add(child, route[i+1:], item)
  }

  child := newNode(nil)
  children[token] = child
  return add(child, route[i+1:], item)
 }

 children := nd.getChildren(route)
 if child, ok := children[route]; ok {
  if child.item != nil {
   return errDupItem
  }

  child.item = item
 } else {
  children[route] = newNode(item)
 }

 return nil
}

func addParam(result *Result, k, v string) {
 if result.Params == nil {
  result.Params = make(map[string]string)
 }

 result.Params[k] = v
}

func duplicatedItem(item string) error {
 return fmt.Errorf("duplicated item for %s", item)
}

func duplicatedSlash(item string) error {
 return fmt.Errorf("duplicated slash for %s", item)
}

func match(pat, token string) innerResult {
 if pat[0] == colon {
  return innerResult{
   key:   pat[1:],
   value: token,
   named: true,
   found: true,
  }
 }

 return innerResult{
  found: pat == token,
 }
}

func newNode(item any) *node {
 return &node{
  item: item,
  children: [2]map[string]*node{
   make(map[string]*node),
   make(map[string]*node),
  },
 }
}
```
