### 代码加上注释版本

```lua
-- 为了兼容阿里云Redis，我们不能使用 `local key = KEYS[1]` 来重复使用键
-- KEYS[1] 作为 tokens_key
-- KEYS[2] 作为 timestamp_key

-- 获取参数
local rate = tonumber(ARGV[1])  -- 速率
local capacity = tonumber(ARGV[2])  -- 容量
local now = tonumber(ARGV[3])  -- 当前时间
local requested = tonumber(ARGV[4])  -- 请求的令牌数量

-- 计算填充时间和TTL
local fill_time = capacity / rate
local ttl = math.floor(fill_time * 2)

-- 获取上次剩余的令牌数量
local last_tokens = tonumber(redis.call("get", KEYS[1]))
if last_tokens == nil then
    last_tokens = capacity
end

-- 获取上次刷新时间
local last_refreshed = tonumber(redis.call("get", KEYS[2]))
if last_refreshed == nil then
    last_refreshed = 0
end

-- 计算时间差和填充的令牌数量
local delta = math.max(0, now - last_refreshed)
local filled_tokens = math.min(capacity, last_tokens + (delta * rate))

-- 判断是否允许请求
local allowed = filled_tokens >= requested
local new_tokens = filled_tokens
if allowed then
    new_tokens = filled_tokens - requested
end

-- 设置新的令牌数量和刷新时间
redis.call("setex", KEYS[1], ttl, new_tokens)
redis.call("setex", KEYS[2], ttl, now)

-- 返回是否允许请求
return allowed
```
