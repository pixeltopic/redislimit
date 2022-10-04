package redislimit

// rateLimitScript manages buckets and computes the number of tokens captured in the provided window of time.
// It automatically expires stale buckets.
// Implementation is O(N) depending on size of buckets within the hash set.
// language=lua
const rateLimitScript = `
	local hash_key = KEYS[1]
	local current_ts = tonumber(ARGV[1]) -- current time, untruncated as unix ts
	local start_trunc_ts = tonumber(ARGV[2]) -- start time of window (truncated) as unix ts
	local end_trunc_ts = tonumber(ARGV[3]) -- end time of window (truncated) as unix ts
	local bucket_precision = tostring(ARGV[4]) -- duration for truncation in seconds
	local stale_bucket_age = tonumber(ARGV[5])
	local threshold = tonumber(ARGV[6])

	local key_expiry = end_trunc_ts - start_trunc_ts
	if not current_ts or not start_trunc_ts or not end_trunc_ts or not bucket_precision or not stale_bucket_age or not threshold then
		return -3
	end
	if threshold <= 0 then
		return -2
	end 
	if key_expiry < 0 then 
		return -1
	end
	local get_bucket_key = function(trunc_ts, bucket_precision)
		return trunc_ts .. ':' .. bucket_precision
	end
	local check_suffix = function(str, suffix)
		return str:sub(-string.len(suffix)) == suffix
	end 
	-- return timestamp from a bucket key before the first : character, otherwise returns nil
	local get_ts_from_key = function(bucket_key)
		local idx, _ = string.find(bucket_key, ':')
		if not idx then
			return idx
		end
		return bucket_key:sub(1, idx-1)
	end
	local get_precision_from_key = function(bucket_key)
		local idx, _ = string.find(bucket_key, ':')
		if not idx then
			return idx
		end
		return bucket_key:sub(idx+1)
	end
	local exists = redis.call('EXISTS', hash_key)
	if exists ~= 1 then
		redis.call('HINCRBY', hash_key, get_bucket_key(end_trunc_ts, bucket_precision), 1)
		redis.call('EXPIRE', hash_key, key_expiry)
		return 1
	end 
	
	local tokens = 1
	local to_del = {}
	local to_del_len = 0
 
	local convert_arr_to_tbl = function(arr)
		local tbl = {}
		local is_key = true
		for idx, v in ipairs(arr) do
			if is_key then 
				tbl[arr[idx]] = arr[idx+1]
			end
			is_key = not is_key
		end
		return tbl
	end
 
	for bucket_key, cnt in pairs(convert_arr_to_tbl(redis.call('HGETALL', hash_key))) do
		local v = tonumber(get_ts_from_key(bucket_key))
		if not v then
			table.insert(to_del, bucket_key)
			to_del_len = to_del_len + 1
		elseif v and stale_bucket_age and v + stale_bucket_age < current_ts then
			table.insert(to_del, bucket_key)
			to_del_len = to_del_len + 1
		elseif check_suffix(bucket_key, bucket_precision) then -- skip anything with incorrect precision
			if v and (v + key_expiry) < current_ts then
				table.insert(to_del, bucket_key)
				to_del_len = to_del_len + 1
			elseif v and v >= start_trunc_ts and v <= end_trunc_ts then
				tokens = tokens + tonumber(cnt)
			end
		end
	end
 
	if to_del_len > 0 then
		redis.call('HDEL', hash_key, unpack(to_del))
	end
 
	local ttl = redis.call('TTL', hash_key)
	if ttl < key_expiry then
		redis.call('EXPIRE', hash_key, key_expiry)
	end

	if tokens <= threshold then
		redis.call('HINCRBY', hash_key, get_bucket_key(end_trunc_ts, bucket_precision), 1) 
		return 1
	end
	return 0
`
