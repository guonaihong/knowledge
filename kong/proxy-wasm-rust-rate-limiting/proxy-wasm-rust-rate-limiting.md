```rust
use chrono::{DateTime, Datelike, Timelike, Utc}; // 引入 chrono 库中的日期时间处理模块
use log::{info, warn, error}; // 引入 log 库中的日志记录模块
use proxy_wasm::{traits::*, types::*}; // 引入 proxy_wasm 库中的特性和类型
use serde::Deserialize; // 引入 serde 库中的反序列化模块
use serde_json_wasm::de; // 引入 serde_json_wasm 库中的反序列化模块
use std::collections::HashMap; // 引入标准库中的哈希映射模块
use std::convert::TryInto; // 引入标准库中的类型转换模块
use std::cmp::max; // 引入标准库中的比较模块
use phf; // 引入 phf 库

// -----------------------------------------------------------------------------
// 配置
// -----------------------------------------------------------------------------

# [derive(Deserialize, Clone, Debug)]
struct Config {
    #[serde(skip_serializing_if = "Option::is_none")]
    second: Option<i32>, // 秒级限制
    #[serde(skip_serializing_if = "Option::is_none")]
    minute: Option<i32>, // 分钟级限制
    #[serde(skip_serializing_if = "Option::is_none")]
    hour: Option<i32>, // 小时级限制
    #[serde(skip_serializing_if = "Option::is_none")]
    day: Option<i32>, // 天级限制
    #[serde(skip_serializing_if = "Option::is_none")]
    month: Option<i32>, // 月级限制
    #[serde(skip_serializing_if = "Option::is_none")]
    year: Option<i32>, // 年级限制
    #[serde(default = "default_limit_by")]
    limit_by: String, // 限制依据
    #[serde(default = "default_empty")]
    header_name: String, // 请求头名称
    #[serde(default = "default_empty")]
    path: String, // 请求路径
    #[serde(default = "default_policy")]
    policy: String, // 策略
    #[serde(default = "default_true")]
    fault_tolerant: bool, // 容错性
    #[serde(default = "default_false")]
    hide_client_headers: bool, // 隐藏客户端请求头
    #[serde(default = "default_429")]
    error_code: u32, // 错误代码
    #[serde(default = "default_msg")]
    error_message: String, // 错误信息
}

fn default_empty() -> String {
    "".to_string()
}

fn default_limit_by() -> String {
    "ip".to_string()
}

fn default_policy() -> String {
    "local".to_string()
}

fn default_true() -> bool {
    true
}

fn default_false() -> bool {
    false
}

fn default_429() -> u32 {
    429
}

fn default_msg() -> String {
    "Rust informs: API rate limit exceeded!".to_string()
}

// -----------------------------------------------------------------------------
// 时间戳
// -----------------------------------------------------------------------------

type TimestampMap = HashMap<&'static str, i64>;

fn get_timestamps(now: DateTime<Utc>) -> TimestampMap {
    let mut ts = TimestampMap::new();

    ts.insert("now", now.timestamp()); // 当前时间戳

    let second = now.with_nanosecond(0).unwrap();
    ts.insert("second", second.timestamp()); // 秒级时间戳

    let minute = second.with_second(0).unwrap();
    ts.insert("minute", minute.timestamp()); // 分钟级时间戳

    let hour = minute.with_minute(0).unwrap();
    ts.insert("hour", hour.timestamp()); // 小时级时间戳

    let day = hour.with_hour(0).unwrap();
    ts.insert("day", day.timestamp()); // 天级时间戳

    let month = day.with_day(1).unwrap();
    ts.insert("month", month.timestamp()); // 月级时间戳

    let year = month.with_month(1).unwrap();
    ts.insert("year", year.timestamp()); // 年级时间戳

    ts
}

// -----------------------------------------------------------------------------
// 根上下文
// -----------------------------------------------------------------------------

static EXPIRATION: phf::Map<&'static str, i32> = phf::phf_map! {
    "second" => 1,
    "minute" => 60,
    "hour" => 3600,
    "day" => 86400,
    "month" => 2592000,
    "year" => 31536000,
};

static X_RATE_LIMIT_LIMIT: phf::Map<&'static str, &'static str> = phf::phf_map! {
    "second" => "X-RateLimit-Limit-Second",
    "minute" => "X-RateLimit-Limit-Minute",
    "hour" => "X-RateLimit-Limit-Hour",
    "day" => "X-RateLimit-Limit-Day",
    "month" => "X-RateLimit-Limit-Month",
    "year" => "X-RateLimit-Limit-Year",
};

static X_RATE_LIMIT_REMAINING: phf::Map<&'static str, &'static str> = phf::phf_map! {
    "second" => "X-RateLimit-Remaining-Second",
    "minute" => "X-RateLimit-Remaining-Minute",
    "hour" => "X-RateLimit-Remaining-Hour",
    "day" => "X-RateLimit-Remaining-Day",
    "month" => "X-RateLimit-Remaining-Month",
    "year" => "X-RateLimit-Remaining-Year",
};

proxy_wasm::main! {{

    proxy_wasm::set_log_level(LogLevel::Debug);
    proxy_wasm::set_root_context(|_| -> Box<dyn RootContext> {
        Box::new(RateLimitingRoot {
            config: None,
        })
    });
}}

struct RateLimitingRoot {
    config: Option<Config>,
}

impl Context for RateLimitingRoot {}
impl RootContext for RateLimitingRoot {
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }

    fn on_configure(&mut self, config_size: usize) -> bool {
        info!("on_configure: config_size: {}", config_size);

        if let Some(config_bytes) = self.get_plugin_configuration() {
            // assert!(config_bytes.len() == config_size);

            match de::from_slice::<Config>(&config_bytes) {
                Ok(config) => {

                    if config.policy != "local" {
                        error!("on_configure: only the local policy is supported for now");
                        return false;
                    }

                    self.config = Some(config);

                    info!("on_configure: loaded configuration: {:?}", self.config);
                    true
                }
                Err(err) => {
                    warn!(
                        "on_configure: failed parsing configuration: {}: {}",
                        String::from_utf8(config_bytes).unwrap(),
                        err
                    );
                    false
                }
            }
        } else {
            warn!("on_configure: failed getting configuration");
            false
        }
    }

    fn create_http_context(&self, context_id: u32) -> Option<Box<dyn HttpContext>> {
        info!("create_http_context: configuration: context_id: {} | {:?}", context_id, self.config);
        if let Some(config) = &self.config {
            let mut limits = HashMap::<&'static str, Option<i32>>::new();

            limits.insert("second", config.second);
            limits.insert("minute", config.minute);
            limits.insert("hour", config.hour);
            limits.insert("day", config.day);
            limits.insert("month", config.month);
            limits.insert("year", config.year);

            Some(Box::new(RateLimitingHttp {
                _context_id: context_id,
                config: config.clone(),
                limits: limits,
                headers: None,
            }))
        } else {
            None
        }
    }
}

// -----------------------------------------------------------------------------
// 插件上下文
// -----------------------------------------------------------------------------

struct RateLimitingHttp {
    _context_id: u32,
    config: Config,
    limits: HashMap<&'static str, Option<i32>>,
    headers: Option<HashMap<&'static str, String>>,
}

struct Usage {
    limit: i32,
    remaining: i32,
    usage: i32,
    cas: Option<u32>,
}

type UsageMap = HashMap<&'static str, Usage>;

# [derive(Default)]
struct Usages {
    counters: Option<UsageMap>,
    stop: Option<&'static str>,
    err: Option<String>,
}

trait RateLimitingPolicy {
    fn usage(&self, id: &str, period: &'static str, ts: &TimestampMap) -> Result<(i32, Option<u32>), String>;

    fn increment(&mut self, id: &str, counters: &UsageMap, ts: &TimestampMap);
}

// 本地策略实现:
impl RateLimitingPolicy for RateLimitingHttp {
    // usage 函数用于获取指定标识符和时间段的使用情况。
    // 它首先生成一个缓存键，然后尝试从共享数据中获取该键对应的数据。
    // 如果数据存在，则将其转换为 i32 类型并返回，同时返回一个 CAS（Compare-And-Swap）值，用于后续的原子操作。
    // 如果数据不存在，则返回 0 和 CAS 值。
    // 由于 proxy-wasm-rust-sdk 在处理错误时会 panic 并将 Status::NotFound 转换为 (None, cas)，因此该函数永远不会返回错误。
    fn usage(&self, id: &str, period: &'static str, ts: &TimestampMap) -> Result<(i32, Option<u32>), String> {
        // 生成缓存键
        let cache_key = self.get_local_key(id, period, ts[period]);
        
        // 尝试获取共享数据
        match self.get_shared_data(&cache_key) {
            // 如果数据存在，将其转换为 i32 并返回
            (Some(data), cas) => {
                Ok((i32::from_le_bytes(data.try_into().unwrap_or_else(|_| [0, 0, 0, 0])), cas))
            }
            // 如果数据不存在，返回 0 和 CAS 值
            (None, cas) => {
                Ok((0, cas))
            }
            // proxy-wasm-rust-sdk 在错误时会 panic 并转换
            // Status::NotFound 为 (None, cas)，
            // 因此此函数永远不会返回 Err
        }
    }

    // increment 函数用于增加指定标识符和时间段的使用计数。
    // 它遍历计数器，为每个时间段生成缓存键，并尝试使用 CAS 操作更新共享数据。
    // 如果 CAS 操作失败（即 CAS 不匹配），它会重新获取当前的使用情况和 CAS 值，并重试最多 10 次。如果最终未能成功更新计数，则记录错误日志。
    fn increment(&mut self, id: &str, counters: &UsageMap, ts: &TimestampMap) {
        // 遍历计数器
        for (period, usage) in counters {
            // 生成缓存键
            let cache_key = self.get_local_key(id, period, ts[period]);
            // 获取当前使用情况和 CAS 值
            let mut value = usage.usage;
            let mut cas = usage.cas;

            // 初始化保存标志
            let mut saved = false;
            // 尝试最多 10 次更新共享数据
            for _ in 0..10 {
                // 将值加一并转换为字节数组
                let buf = (value + 1).to_le_bytes();
                // 尝试设置共享数据
                match self.set_shared_data(&cache_key, Some(&buf), cas) {
                    Ok(()) => {
                        // 如果设置成功，标记为已保存并退出循环
                        saved = true;
                        break;
                    }
                    Err(Status::CasMismatch) => {
                        // 如果 CAS 不匹配，重新获取使用情况和 CAS 值
                        if let Ok((nvalue, ncas)) = self.usage(id, period, ts) {
                            if ncas != None {
                                value = nvalue;
                                cas = ncas;
                            }
                        }
                    }
                    Err(_) => {
                        // 任何其他情况都会导致 proxy-wasm-rust-sdk 恐慌。
                    }
                }
            }

            // 如果未能保存，记录错误日志
            if !saved {
                log::error!("could not increment counter for period '{}'", period)
            }
        }
    }
}

impl RateLimitingHttp {
    fn get_prop(&self, ns: &str, prop: &str) -> String {
        if let Some(addr) = self.get_property(vec![ns, prop]) {
            match std::str::from_utf8(&addr) {
                Ok(value) => value.to_string(),
                Err(_) => "".to_string(),
            }
        } else {
            "".to_string()
        }
    }

    fn get_identifier(&self) -> String {
        match self.config.limit_by.as_str() {
            "header" => {
                if let Some(header) = self.get_http_request_header(&self.config.header_name) {
                    return header.to_string();
                }
            }
            "path" => {
                if let Some(path) = self.get_http_request_header(":path") {
                    if path == self.config.path {
                        return path.to_string();
                    }
                }
            }
            &_ => {}
        }

        // "ip" 是回退选项:
        return self.get_prop("ngx", "remote_addr");
    }

    fn get_local_key(&self, id: &str, period: &'static str, date: i64) -> String {
        format!("kong_wasm_rate_limiting_counters/ratelimit:{}:{}:{}:{}:{}",
            self.get_prop("kong", "route_id"),
            self.get_prop("kong", "service_id"),
            id, date, period)
    }

    // get_usages 函数用于获取指定标识符在各个时间段的使用情况。
    // 它遍历限制配置，为每个时间段调用 usage 函数获取当前的使用情况，并计算剩余使用情况。
    // 如果某个时间段的剩余使用情况小于等于零，则设置停止标志。
    // 如果获取使用情况失败，则设置错误标志并跳出循环。最后，将计数器哈希映射插入 Usages 结构体并返回。
    fn get_usages(&mut self, id: &str, ts: &TimestampMap) -> Usages {
        // 初始化 Usages 结构体
        let mut usages: Usages = Default::default();
        // 初始化计数器哈希映射
        let mut counters = UsageMap::new();

        // 遍历限制配置
        for (period, &limit) in &self.limits {
            // 如果限制存在
            if let Some(limit) = limit {
                // 获取当前使用情况
                match self.usage(id, period, ts) {
                    Ok((cur_usage, cas)) => {
                        // 计算剩余使用情况
                        let remaining = limit - cur_usage;

                        // 将使用情况插入计数器哈希映射
                        counters.insert(period, Usage {
                            limit: limit,
                            remaining: remaining,
                            usage: cur_usage,
                            cas: cas,
                        });

                        // 如果剩余使用情况小于等于零，设置停止标志
                        if remaining <= 0 {
                            usages.stop = Some(period);
                        }
                    }
                    Err(err) => {
                        // 如果获取使用情况失败，设置错误标志并跳出循环
                        usages.err = Some(err);
                        break;
                    }
                }
            }
        }

        // 将计数器哈希映射插入 Usages 结构体
        usages.counters = Some(counters);
        // 返回 Usages 结构体
        usages
    }

    // process_usage 函数用于处理使用情况并生成相应的 HTTP 响应头。
    // 它首先检查是否需要隐藏客户端请求头，如果不隐藏，则遍历计数器，计算每个时间段的限制、窗口和剩余情况，并更新响应头。
    // 如果某个时间段的剩余次数为零，则设置停止条件，并发送错误响应，指示客户端稍后重试。如果所有时间段的剩余次数都大于零，则继续处理请求。
    fn process_usage(&mut self, counters: &UsageMap, stop: Option<&'static str>, ts: &TimestampMap) -> Action {
        // 获取当前时间戳
        let now = ts["now"];
        // 初始化重置时间
        let mut reset: i32 = 0;

        // 如果不隐藏客户端请求头
        if !self.config.hide_client_headers {
            // 初始化限制、窗口和剩余变量
            let mut limit: i32 = 0;
            let mut window: i32 = 0;
            let mut remaining: i32 = 0;
            // 初始化响应头哈希映射
            let mut headers = HashMap::<&'static str, String>::new();

            // 遍历计数器
            for (period, usage) in counters {
                // 获取当前限制和窗口
                let cur_limit = usage.limit;
                let cur_window = EXPIRATION[period];
                // 计算当前剩余
                let mut cur_remaining = usage.remaining;

                // 如果停止条件满足，减少剩余
                if stop == None || stop == Some(period) {
                    cur_remaining -= 1;
                }
                // 确保剩余不为负
                cur_remaining = max(0, cur_remaining);

                // 更新限制、窗口和剩余
                if (limit == 0) || (cur_remaining < remaining) || (cur_remaining == remaining && cur_window > window) {
                    limit = cur_limit;
                    window = cur_window;
                    remaining = cur_remaining;

                    // 计算重置时间
                    reset = max(1, window - (now - ts[period]) as i32);
                }

                // 添加限制和剩余到响应头
                headers.insert(X_RATE_LIMIT_LIMIT[period], limit.to_string());
                headers.insert(X_RATE_LIMIT_REMAINING[period], remaining.to_string());
            }

            // 添加总限制、剩余和重置时间到响应头
            headers.insert("RateLimit-Limit", limit.to_string());
            headers.insert("RateLimit-Remaining", remaining.to_string());
            headers.insert("RateLimit-Reset", reset.to_string());

            // 更新响应头
            self.headers = Some(headers);
        }

        // 如果停止条件满足，发送错误响应并暂停
        if stop != None {
            if self.headers == None {
                self.headers = Some(HashMap::new());
            }
            if let Some(headers) = &mut self.headers {
                headers.insert("Retry-After", reset.to_string());
            }

            self.send_http_response(self.config.error_code, vec![], Some(self.config.error_message.as_bytes()));

            Action::Pause
        } else {
            // 否则继续处理
            Action::Continue
        }
    }
}

impl Context for RateLimitingHttp {}

// 这段代码实现了 HTTP 请求和响应的处理逻辑。
// 在 on_http_request_headers 方法中，首先获取当前时间戳，然后根据配置的限制策略获取请求的标识符，并计算当前的使用情况。
// 如果出现错误且配置为不容错，则抛出异常；否则记录错误日志。如果使用情况正常，则根据使用情况处理请求并更新计数器。
// 在 on_http_response_headers 方法中，如果响应头已经完全接收，则将计算出的限制信息添加到响应头中。
impl HttpContext for RateLimitingHttp {
    fn on_http_request_headers(&mut self, _nheaders: usize, _eof: bool) -> Action {
        // 获取当前时间
        let now: DateTime<Utc> = self.get_current_time().into();

        // 获取当前时间戳
        let ts = get_timestamps(now);

        // 获取请求的标识符
        let id = self.get_identifier();

        // 获取当前的使用情况
        let usages = self.get_usages(&id, &ts);

        // 如果有错误，根据容错配置处理错误
        if let Some(err) = usages.err {
            if !self.config.fault_tolerant {
                panic!("{}", err.to_string());
            }
            log::error!("failed to get usage: {}", err);
        }

        // 如果有计数器，处理使用情况并更新计数器
        if let Some(counters) = usages.counters {
            let action = self.process_usage(&counters, usages.stop, &ts);
            if action != Action::Continue {
                return action;
            }

            self.increment(&id, &counters, &ts);
        }

        // 继续处理请求
        Action::Continue
    }

    fn on_http_response_headers(&mut self, _nheaders: usize, eof: bool) -> Action {
        // 如果响应头未完全接收，继续处理
        if !eof {
            return Action::Continue;
        }

        // 如果有计算出的限制信息，将其添加到响应头中
        if let Some(headers) = &self.headers {
            for (k, v) in headers {
                self.add_http_response_header(k, &v);
            }
        }

        // 继续处理响应
        Action::Continue
    }
}
