package redislimit

// handleRateScript manages buckets and computes the number of tokens captured in the provided window of time.
// It automatically expires stale buckets.
// Implementation is O(N) depending on size of buckets within the hash set.
// language=lua
const handleRateScript = `
	local key = KEYS[1]

	local current_ts = tonumber(ARGV[1]) -- current time, untruncated as unix ts
	local start_trunc_ts = tonumber(ARGV[2]) -- start time of window (truncated) as unix ts
	local end_trunc_ts = tonumber(ARGV[3]) -- end time of window (truncated) as unix ts
	local bucket_precision = tostring(ARGV[4]) -- duration for truncation in seconds
	local stale_bucket_age = tonumber(ARGV[5])
	local key_expiry = end_trunc_ts - start_trunc_ts

	if not current_ts or not start_trunc_ts or not end_trunc_ts or not bucket_precision or not stale_bucket_age then
		return -2
	end

	if key_expiry < 0 then 
		return -1
	end

	local get_bucket_key = function(trunc_ts, bucket_precision)
		return trunc_ts .. bucket_precision
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

	local hash_set = redis.call('HGETALL', key)
	if not hash_set then
		local tokens = redis.call('HINCRBY', key, get_bucket_key(end_trunc_ts, bucket_precision, 1)
		redis.call('EXPIRE', key, key_expiry)
		return tokens
	end 

	redis.call('HINCRBY', key, get_bucket_key(end_trunc_ts, bucket_precision, 1)
	local tokens = 1

	local to_del = {}
	for bucket_key, cnt in pairs(hash_set) do
		local v = tonumber(get_ts_from_key(bucket_key))
		local exp = tonumber(get_precision_from_key(bucket_key))

		if not v then
			table.insert(to_del, bucket_key)
		else not exp then
			table.insert(to_del, bucket_key)
		else v and stale_bucket_age and v + stale_bucket_age < current_ts then
			table.insert(to_del, bucket_key)
		else check_suffix(bucket_key, bucket_precision) then -- skip anything with incorrect precision
			if v and exp and v + exp < current_ts then
				table.insert(to_del, bucket_key)
			else v and v >= start_trunc_ts and tonumber(v) <= end_trunc_ts then
				tokens += cnt
			end
		end
	end 

	redis.call('HDEL', key, unpack(to_del))

	redis.call('EXPIRE', key, key_expiry, 'GT')

	return tokens
`
