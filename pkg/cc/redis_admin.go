package cc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

const (
	redisDefaultScanCount     = 250
	redisMaxScanItems         = 5000
	redisMaxStringPreview     = 64 * 1024
	redisMaxCollectionPreview = 5000
	redisMaxDB                = 15
	redisTypeString           = "string"
	redisTypeHash             = "hash"
	redisTypeList             = "list"
	redisTypeSet              = "set"
	redisTypeZSet             = "zset"
)

var (
	errRedisNotFound = errors.New("redis key not found")
)

type redisConflictError struct {
	Current *redisKeyDetailResponse
}

func (e *redisConflictError) Error() string {
	return "key changed since it was loaded"
}

// redisEncodedValue safely represents possibly-binary values.
type redisEncodedValue struct {
	Mode   string `json:"mode"` // "text" | "base64"
	Text   string `json:"text,omitempty"`
	Base64 string `json:"base64,omitempty"`
}

type redisHashEntry struct {
	Field string            `json:"field"`
	Value redisEncodedValue `json:"value"`
}

type redisZSetEntry struct {
	Member redisEncodedValue `json:"member"`
	Score  float64           `json:"score"`
}

type redisKeyMeta struct {
	TotalItems int64 `json:"totalItems,omitempty"`
	SizeBytes  int64 `json:"sizeBytes,omitempty"`
	Truncated  bool  `json:"truncated,omitempty"`
}

type redisKeyDetailResponse struct {
	DB          int                 `json:"db"`
	Key         string              `json:"key"`
	Type        string              `json:"type"`
	TTLMS       int64               `json:"ttlMs"`
	Version     string              `json:"version"`
	Meta        redisKeyMeta        `json:"meta"`
	StringValue *redisEncodedValue  `json:"stringValue,omitempty"`
	HashEntries []redisHashEntry    `json:"hashEntries,omitempty"`
	ListItems   []redisEncodedValue `json:"listItems,omitempty"`
	SetMembers  []redisEncodedValue `json:"setMembers,omitempty"`
	ZSetMembers []redisZSetEntry    `json:"zsetMembers,omitempty"`
}

type redisStatusResponse struct {
	Connected bool   `json:"connected"`
	Addr      string `json:"addr"`
	DB        int    `json:"db"`
	Ping      string `json:"ping"`
	DBSize    int64  `json:"dbSize"`
}

type redisTreeResponse struct {
	DB         int      `json:"db"`
	Prefix     string   `json:"prefix"`
	Count      int      `json:"count"`
	Cursor     uint64   `json:"cursor"`
	NextCursor uint64   `json:"nextCursor"`
	Scanned    int      `json:"scanned"`
	Truncated  bool     `json:"truncated"`
	Branches   []string `json:"branches"`
	Leaves     []string `json:"leaves"`
}

type redisSearchResponse struct {
	DB         int      `json:"db"`
	Query      string   `json:"query"`
	Count      int      `json:"count"`
	Cursor     uint64   `json:"cursor"`
	NextCursor uint64   `json:"nextCursor"`
	Scanned    int      `json:"scanned"`
	Truncated  bool     `json:"truncated"`
	Keys       []string `json:"keys"`
}

type redisWriteRequest struct {
	DB              int                 `json:"db"`
	Key             string              `json:"key"`
	Type            string              `json:"type"`
	ExpectedVersion string              `json:"expectedVersion,omitempty"`
	TTLMode         string              `json:"ttlMode,omitempty"` // "none" | "keep" | "set" | "clear"
	TTLSeconds      int64               `json:"ttlSeconds,omitempty"`
	StringValue     *redisEncodedValue  `json:"stringValue,omitempty"`
	HashEntries     []redisHashEntry    `json:"hashEntries,omitempty"`
	ListItems       []redisEncodedValue `json:"listItems,omitempty"`
	SetMembers      []redisEncodedValue `json:"setMembers,omitempty"`
	ZSetMembers     []redisZSetEntry    `json:"zsetMembers,omitempty"`
}

type redisDeleteManyRequest struct {
	DB   int      `json:"db"`
	Keys []string `json:"keys"`
}

type redisDeleteManyResult struct {
	Key     string `json:"key"`
	Deleted bool   `json:"deleted"`
	Error   string `json:"error,omitempty"`
}

type redisDeleteManyResponse struct {
	DB      int                     `json:"db"`
	Results []redisDeleteManyResult `json:"results"`
}

// RedisAdmin wraps local Redis access for Command Center APIs.
type RedisAdmin struct {
	log      logrus.FieldLogger
	getRedis func() string
}

func newRedisAdmin(log logrus.FieldLogger, getRedisAddr func() string) *RedisAdmin {
	return &RedisAdmin{
		log:      log.WithField("component", "redis-admin"),
		getRedis: getRedisAddr,
	}
}

func (a *RedisAdmin) status(ctx context.Context, db int) (redisStatusResponse, error) {
	addr := a.getRedis()

	client := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   db,
	})
	defer client.Close()

	ping, err := client.Ping(ctx).Result()
	if err != nil {
		return redisStatusResponse{}, err
	}

	size, err := client.DBSize(ctx).Result()
	if err != nil {
		return redisStatusResponse{}, err
	}

	return redisStatusResponse{
		Connected: true,
		Addr:      addr,
		DB:        db,
		Ping:      ping,
		DBSize:    size,
	}, nil
}

func (a *RedisAdmin) tree(
	ctx context.Context,
	db int,
	prefix string,
	cursor uint64,
	count int,
) (redisTreeResponse, error) {
	client := redis.NewClient(&redis.Options{
		Addr: a.getRedis(),
		DB:   db,
	})
	defer client.Close()

	match := "*"
	if prefix != "" {
		match = prefix + "*"
	}

	branches := make(map[string]struct{})
	leaves := make(map[string]struct{})
	scanned := 0
	next := cursor
	truncated := false

	for {
		keys, c, err := client.Scan(ctx, next, match, int64(count)).Result()
		if err != nil {
			return redisTreeResponse{}, err
		}

		scanned += len(keys)

		for _, key := range keys {
			branch, leaf, ok := splitKeyByPrefix(prefix, key)
			if !ok {
				continue
			}

			if branch != "" {
				branches[branch] = struct{}{}
			} else if leaf != "" {
				leaves[leaf] = struct{}{}
			}

			if len(branches)+len(leaves) >= redisMaxScanItems {
				truncated = true

				break
			}
		}

		next = c

		if next == 0 || truncated {
			break
		}
	}

	branchList := sortedKeys(branches)
	leafList := sortedKeys(leaves)

	return redisTreeResponse{
		DB:         db,
		Prefix:     prefix,
		Count:      count,
		Cursor:     cursor,
		NextCursor: next,
		Scanned:    scanned,
		Truncated:  truncated,
		Branches:   branchList,
		Leaves:     leafList,
	}, nil
}

func (a *RedisAdmin) search(
	ctx context.Context,
	db int,
	query string,
	cursor uint64,
	count int,
) (redisSearchResponse, error) {
	client := redis.NewClient(&redis.Options{
		Addr: a.getRedis(),
		DB:   db,
	})
	defer client.Close()

	match := "*"
	if query != "" {
		match = "*" + query + "*"
	}

	keys := make(map[string]struct{})
	scanned := 0
	next := cursor
	truncated := false

	for {
		page, c, err := client.Scan(ctx, next, match, int64(count)).Result()
		if err != nil {
			return redisSearchResponse{}, err
		}

		scanned += len(page)

		for _, key := range page {
			keys[key] = struct{}{}
			if len(keys) >= redisMaxScanItems {
				truncated = true

				break
			}
		}

		next = c
		if next == 0 || truncated {
			break
		}
	}

	return redisSearchResponse{
		DB:         db,
		Query:      query,
		Count:      count,
		Cursor:     cursor,
		NextCursor: next,
		Scanned:    scanned,
		Truncated:  truncated,
		Keys:       sortedKeys(keys),
	}, nil
}

func (a *RedisAdmin) getKey(
	ctx context.Context,
	db int,
	key string,
) (redisKeyDetailResponse, error) {
	client := redis.NewClient(&redis.Options{
		Addr: a.getRedis(),
		DB:   db,
	})
	defer client.Close()

	detail, err := readRedisKey(ctx, client, db, key)
	if err != nil {
		return redisKeyDetailResponse{}, err
	}

	return detail, nil
}

func (a *RedisAdmin) createKey(ctx context.Context, req redisWriteRequest) (redisKeyDetailResponse, error) {
	client := redis.NewClient(&redis.Options{
		Addr: a.getRedis(),
		DB:   req.DB,
	})
	defer client.Close()

	err := validateWriteRequest(req, true)
	if err != nil {
		return redisKeyDetailResponse{}, err
	}

	err = client.Watch(ctx, func(tx *redis.Tx) error {
		exists, existsErr := tx.Exists(ctx, req.Key).Result()
		if existsErr != nil {
			return existsErr
		}

		if exists > 0 {
			return &redisConflictError{}
		}

		_, pipeErr := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			return applyWrite(ctx, pipe, req, -1)
		})

		return pipeErr
	}, req.Key)
	if err != nil {
		return redisKeyDetailResponse{}, err
	}

	return readRedisKey(ctx, client, req.DB, req.Key)
}

func (a *RedisAdmin) updateKey(ctx context.Context, req redisWriteRequest) (redisKeyDetailResponse, error) {
	client := redis.NewClient(&redis.Options{
		Addr: a.getRedis(),
		DB:   req.DB,
	})
	defer client.Close()

	err := validateWriteRequest(req, false)
	if err != nil {
		return redisKeyDetailResponse{}, err
	}

	var latest redisKeyDetailResponse

	err = client.Watch(ctx, func(tx *redis.Tx) error {
		current, readErr := readRedisKey(ctx, tx, req.DB, req.Key)
		if readErr != nil {
			return readErr
		}

		latest = current
		if req.ExpectedVersion != "" && req.ExpectedVersion != current.Version {
			return &redisConflictError{Current: &current}
		}

		ttl := current.TTLMS
		_, pipeErr := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			return applyWrite(ctx, pipe, req, ttl)
		})

		return pipeErr
	}, req.Key)
	if err != nil {
		var conflict *redisConflictError
		if errors.As(err, &conflict) && conflict.Current == nil {
			conflict.Current = &latest
		}

		return redisKeyDetailResponse{}, err
	}

	return readRedisKey(ctx, client, req.DB, req.Key)
}

func (a *RedisAdmin) deleteKey(ctx context.Context, db int, key string) (bool, error) {
	client := redis.NewClient(&redis.Options{
		Addr: a.getRedis(),
		DB:   db,
	})
	defer client.Close()

	deleted, err := client.Del(ctx, key).Result()
	if err != nil {
		return false, err
	}

	return deleted > 0, nil
}

func (a *RedisAdmin) deleteKeys(ctx context.Context, req redisDeleteManyRequest) (redisDeleteManyResponse, error) {
	client := redis.NewClient(&redis.Options{
		Addr: a.getRedis(),
		DB:   req.DB,
	})
	defer client.Close()

	results := make([]redisDeleteManyResult, 0, len(req.Keys))

	for _, key := range req.Keys {
		if strings.TrimSpace(key) == "" {
			results = append(results, redisDeleteManyResult{
				Key:     key,
				Deleted: false,
				Error:   "key is required",
			})

			continue
		}

		deleted, err := client.Del(ctx, key).Result()
		if err != nil {
			results = append(results, redisDeleteManyResult{
				Key:     key,
				Deleted: false,
				Error:   err.Error(),
			})

			continue
		}

		results = append(results, redisDeleteManyResult{
			Key:     key,
			Deleted: deleted > 0,
		})
	}

	return redisDeleteManyResponse{
		DB:      req.DB,
		Results: results,
	}, nil
}

func validateWriteRequest(req redisWriteRequest, isCreate bool) error {
	if req.Key == "" {
		return errors.New("key is required")
	}

	switch req.Type {
	case redisTypeString:
		if req.StringValue == nil {
			return errors.New("stringValue is required for string keys")
		}
	case redisTypeHash:
		if len(req.HashEntries) == 0 {
			return errors.New("hashEntries must contain at least one entry")
		}
	case redisTypeList:
		if len(req.ListItems) == 0 {
			return errors.New("listItems must contain at least one item")
		}
	case redisTypeSet:
		if len(req.SetMembers) == 0 {
			return errors.New("setMembers must contain at least one member")
		}
	case redisTypeZSet:
		if len(req.ZSetMembers) == 0 {
			return errors.New("zsetMembers must contain at least one member")
		}
	default:
		return fmt.Errorf("unsupported key type: %s", req.Type)
	}

	if req.TTLMode == "set" && req.TTLSeconds <= 0 {
		return errors.New("ttlSeconds must be greater than 0 when ttlMode is set")
	}

	if !isCreate && req.ExpectedVersion == "" {
		return errors.New("expectedVersion is required for updates")
	}

	return nil
}

func applyWrite(ctx context.Context, pipe redis.Pipeliner, req redisWriteRequest, priorTTLMS int64) error {
	pipe.Del(ctx, req.Key)

	switch req.Type {
	case redisTypeString:
		value, err := decodeValue(*req.StringValue)
		if err != nil {
			return err
		}

		pipe.Set(ctx, req.Key, value, 0)
	case redisTypeHash:
		args := make([]any, 0, len(req.HashEntries)*2)
		for _, entry := range req.HashEntries {
			value, err := decodeValue(entry.Value)
			if err != nil {
				return err
			}

			args = append(args, entry.Field, value)
		}

		pipe.HSet(ctx, req.Key, args...)
	case redisTypeList:
		values := make([]any, 0, len(req.ListItems))
		for _, item := range req.ListItems {
			value, err := decodeValue(item)
			if err != nil {
				return err
			}

			values = append(values, value)
		}

		pipe.RPush(ctx, req.Key, values...)
	case redisTypeSet:
		values := make([]any, 0, len(req.SetMembers))
		for _, member := range req.SetMembers {
			value, err := decodeValue(member)
			if err != nil {
				return err
			}

			values = append(values, value)
		}

		pipe.SAdd(ctx, req.Key, values...)
	case redisTypeZSet:
		values := make([]redis.Z, 0, len(req.ZSetMembers))
		for _, member := range req.ZSetMembers {
			value, err := decodeValue(member.Member)
			if err != nil {
				return err
			}

			values = append(values, redis.Z{
				Member: value,
				Score:  member.Score,
			})
		}

		pipe.ZAdd(ctx, req.Key, values...)
	default:
		return fmt.Errorf("unsupported key type: %s", req.Type)
	}

	mode := req.TTLMode
	if mode == "" {
		mode = "keep"
	}

	switch mode {
	case "keep":
		if priorTTLMS > 0 {
			pipe.PExpire(ctx, req.Key, time.Duration(priorTTLMS)*time.Millisecond)
		}
	case redisTypeSet:
		pipe.Expire(ctx, req.Key, time.Duration(req.TTLSeconds)*time.Second)
	case "clear", "none":
		pipe.Persist(ctx, req.Key)
	default:
		return fmt.Errorf("invalid ttlMode: %s", mode)
	}

	return nil
}

func readRedisKey(
	ctx context.Context,
	client redis.Cmdable,
	db int,
	key string,
) (redisKeyDetailResponse, error) {
	t, err := client.Type(ctx, key).Result()
	if err != nil {
		return redisKeyDetailResponse{}, err
	}

	if t == "none" {
		return redisKeyDetailResponse{}, errRedisNotFound
	}

	ttl, err := client.PTTL(ctx, key).Result()
	if err != nil {
		return redisKeyDetailResponse{}, err
	}

	resp := redisKeyDetailResponse{
		DB:    db,
		Key:   key,
		Type:  t,
		TTLMS: normalizeTTL(ttl),
	}

	switch t {
	case redisTypeString:
		size, sizeErr := client.StrLen(ctx, key).Result()
		if sizeErr != nil {
			return redisKeyDetailResponse{}, sizeErr
		}

		value, getErr := client.GetRange(ctx, key, 0, redisMaxStringPreview-1).Result()
		if getErr != nil && !errors.Is(getErr, redis.Nil) {
			return redisKeyDetailResponse{}, getErr
		}

		enc := encodeValue(value)
		resp.StringValue = &enc
		resp.Meta.SizeBytes = size
		resp.Meta.Truncated = size > redisMaxStringPreview
	case redisTypeHash:
		total, totalErr := client.HLen(ctx, key).Result()
		if totalErr != nil {
			return redisKeyDetailResponse{}, totalErr
		}

		entries, truncated, readErr := readHashEntries(ctx, client, key)
		if readErr != nil {
			return redisKeyDetailResponse{}, readErr
		}

		resp.HashEntries = entries
		resp.Meta.TotalItems = total
		resp.Meta.Truncated = truncated
	case redisTypeList:
		total, totalErr := client.LLen(ctx, key).Result()
		if totalErr != nil {
			return redisKeyDetailResponse{}, totalErr
		}

		items, listErr := client.LRange(ctx, key, 0, redisMaxCollectionPreview-1).Result()
		if listErr != nil {
			return redisKeyDetailResponse{}, listErr
		}

		resp.ListItems = make([]redisEncodedValue, 0, len(items))
		for _, item := range items {
			resp.ListItems = append(resp.ListItems, encodeValue(item))
		}

		resp.Meta.TotalItems = total
		resp.Meta.Truncated = total > redisMaxCollectionPreview
	case redisTypeSet:
		total, totalErr := client.SCard(ctx, key).Result()
		if totalErr != nil {
			return redisKeyDetailResponse{}, totalErr
		}

		members, truncated, setErr := readSetMembers(ctx, client, key)
		if setErr != nil {
			return redisKeyDetailResponse{}, setErr
		}

		resp.SetMembers = members
		resp.Meta.TotalItems = total
		resp.Meta.Truncated = truncated
	case redisTypeZSet:
		total, totalErr := client.ZCard(ctx, key).Result()
		if totalErr != nil {
			return redisKeyDetailResponse{}, totalErr
		}

		zs, zErr := client.ZRangeWithScores(ctx, key, 0, redisMaxCollectionPreview-1).Result()
		if zErr != nil {
			return redisKeyDetailResponse{}, zErr
		}

		resp.ZSetMembers = make([]redisZSetEntry, 0, len(zs))
		for _, z := range zs {
			member := fmt.Sprintf("%v", z.Member)
			resp.ZSetMembers = append(resp.ZSetMembers, redisZSetEntry{
				Member: encodeValue(member),
				Score:  z.Score,
			})
		}

		resp.Meta.TotalItems = total
		resp.Meta.Truncated = total > redisMaxCollectionPreview
	default:
		return redisKeyDetailResponse{}, fmt.Errorf("unsupported redis type: %s", t)
	}

	resp.Version = buildVersionToken(resp)

	return resp, nil
}

func readHashEntries(
	ctx context.Context,
	client redis.Cmdable,
	key string,
) ([]redisHashEntry, bool, error) {
	cursor := uint64(0)
	entries := make([]redisHashEntry, 0, 128)
	truncated := false

	for {
		page, next, err := client.HScan(ctx, key, cursor, "*", int64(redisDefaultScanCount)).Result()
		if err != nil {
			return nil, false, err
		}

		for i := 0; i+1 < len(page); i += 2 {
			entries = append(entries, redisHashEntry{
				Field: page[i],
				Value: encodeValue(page[i+1]),
			})

			if len(entries) >= redisMaxCollectionPreview {
				truncated = true

				break
			}
		}

		cursor = next
		if cursor == 0 || truncated {
			break
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Field < entries[j].Field
	})

	return entries, truncated, nil
}

func readSetMembers(
	ctx context.Context,
	client redis.Cmdable,
	key string,
) ([]redisEncodedValue, bool, error) {
	cursor := uint64(0)
	members := make([]redisEncodedValue, 0, 128)
	truncated := false

	for {
		page, next, err := client.SScan(ctx, key, cursor, "*", int64(redisDefaultScanCount)).Result()
		if err != nil {
			return nil, false, err
		}

		for _, member := range page {
			members = append(members, encodeValue(member))
			if len(members) >= redisMaxCollectionPreview {
				truncated = true

				break
			}
		}

		cursor = next
		if cursor == 0 || truncated {
			break
		}
	}

	sort.Slice(members, func(i, j int) bool {
		return encodedSortValue(members[i]) < encodedSortValue(members[j])
	})

	return members, truncated, nil
}

func encodeValue(value string) redisEncodedValue {
	if utf8.ValidString(value) {
		return redisEncodedValue{
			Mode: "text",
			Text: value,
		}
	}

	return redisEncodedValue{
		Mode:   "base64",
		Base64: base64.StdEncoding.EncodeToString([]byte(value)),
	}
}

func decodeValue(value redisEncodedValue) (string, error) {
	switch value.Mode {
	case "", "text":
		return value.Text, nil
	case "base64":
		data, err := base64.StdEncoding.DecodeString(value.Base64)
		if err != nil {
			return "", fmt.Errorf("invalid base64 value: %w", err)
		}

		return string(data), nil
	default:
		return "", fmt.Errorf("unsupported value mode: %s", value.Mode)
	}
}

func splitKeyByPrefix(prefix, key string) (branch string, leaf string, ok bool) {
	if prefix != "" && !strings.HasPrefix(key, prefix) {
		return "", "", false
	}

	remaining := strings.TrimPrefix(key, prefix)
	if remaining == "" {
		return "", key, true
	}

	idx := strings.Index(remaining, ":")
	if idx == -1 {
		return "", prefix + remaining, true
	}

	return prefix + remaining[:idx+1], "", true
}

func sortedKeys(values map[string]struct{}) []string {
	list := make([]string, 0, len(values))
	for value := range values {
		list = append(list, value)
	}

	sort.Strings(list)

	return list
}

func buildVersionToken(detail redisKeyDetailResponse) string {
	var b strings.Builder

	b.WriteString(detail.Type)
	b.WriteString("|")
	b.WriteString(strconv.FormatInt(detail.TTLMS, 10))
	b.WriteString("|")

	switch detail.Type {
	case redisTypeString:
		if detail.StringValue != nil {
			b.WriteString(encodedSortValue(*detail.StringValue))
		}
	case redisTypeHash:
		for _, entry := range detail.HashEntries {
			b.WriteString(entry.Field)
			b.WriteString("=")
			b.WriteString(encodedSortValue(entry.Value))
			b.WriteString(";")
		}
	case redisTypeList:
		for _, item := range detail.ListItems {
			b.WriteString(encodedSortValue(item))
			b.WriteString(";")
		}
	case redisTypeSet:
		values := make([]string, 0, len(detail.SetMembers))
		for _, member := range detail.SetMembers {
			values = append(values, encodedSortValue(member))
		}

		sort.Strings(values)

		for _, value := range values {
			b.WriteString(value)
			b.WriteString(";")
		}
	case redisTypeZSet:
		values := make([]string, 0, len(detail.ZSetMembers))
		for _, member := range detail.ZSetMembers {
			values = append(values, encodedSortValue(member.Member)+":"+strconv.FormatFloat(member.Score, 'g', -1, 64))
		}

		sort.Strings(values)

		for _, value := range values {
			b.WriteString(value)
			b.WriteString(";")
		}
	}

	hash := sha256.Sum256([]byte(b.String()))

	return hex.EncodeToString(hash[:])
}

func encodedSortValue(value redisEncodedValue) string {
	if value.Mode == "base64" {
		return "b64:" + value.Base64
	}

	return "txt:" + value.Text
}

func normalizeTTL(ttl time.Duration) int64 {
	if ttl == -1 {
		return -1
	}

	if ttl == -2 {
		return -2
	}

	return ttl.Milliseconds()
}
