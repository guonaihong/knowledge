### 代码加上注释版本

```lua
-- 由于与阿里云Redis兼容性问题，我们不能使用 `local key = KEYS[1]` 来重用键
local limit = tonumber(ARGV[1])  -- 获取限制值，转换为数字
local window = tonumber(ARGV[2])  -- 获取时间窗口，转换为数字
local current = redis.call("INCRBY", KEYS[1], 1)  -- 将键的值增加1，并获取当前值

if current == 1 then
    -- 如果当前值为1，表示这是第一次设置该键，设置过期时间
    redis.call("expire", KEYS[1], window)
end

if current < limit then
    -- 如果当前值小于限制值，返回1表示允许
    return 1
elseif current == limit then
    -- 如果当前值等于限制值，返回2表示达到配额
    return 2
else
    -- 如果当前值大于限制值，返回0表示超过配额
    return 0
end
```
