
## 鉴权是怎么实现的？

鉴权分为生成和鉴权两步。

* 生成: 使用短信一键登录换取服务端的jwt token
* 鉴权: 服务端校验jwt token是否过期，如果过期则直接返回错误，以及签名的合法性
token通常放入grpc 的Authorization或者http的Authorization字段中，

参考资料

* <https://www.ruanyifeng.com/blog/2018/07/json_web_token-tutorial.html>
* <https://en.wikipedia.org/wiki/JSON_Web_Token>

jwt 数据结构由三部分组成

* Header（头部）
* Payload（负载）
* Signature（签名）

### header组成

```json
{
  "alg": "HS256",
  "typ": "JWT"
}
```

### payload

* iss (issuer)：签发人
* exp (expiration time)：过期时间
* sub (subject)：主题
* aud (audience)：受众
* nbf (Not Before)：生效时间
* iat (Issued At)：签发时间
* jti (JWT ID)：编号

签名计算方式

```console
HMAC_SHA256(
  secret,
  base64urlEncoding(header) + '.' +
  base64urlEncoding(payload)
)
```

jwt 编码过程

```js
const token = base64urlEncoding(header) + '.' + base64urlEncoding(payload) + '.' + base64urlEncoding(signature)
```

* token拉黑如何实现
一般是存放redis里面
