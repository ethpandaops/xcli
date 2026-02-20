package cc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (a *apiHandler) handleGetRedisStatus(w http.ResponseWriter, r *http.Request) {
	db, err := parseRedisDB(r.URL.Query().Get("db"))
	if err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid db", err)

		return
	}

	resp, err := a.redis.status(r.Context(), db)
	if err != nil {
		writeRedisError(w, http.StatusInternalServerError, "redis status failed", err)

		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *apiHandler) handleGetRedisTree(w http.ResponseWriter, r *http.Request) {
	db, err := parseRedisDB(r.URL.Query().Get("db"))
	if err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid db", err)

		return
	}

	cursor, err := parseRedisCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid cursor", err)

		return
	}

	count, err := parseRedisCount(r.URL.Query().Get("count"))
	if err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid count", err)

		return
	}

	prefix := strings.TrimSpace(r.URL.Query().Get("prefix"))

	resp, err := a.redis.tree(r.Context(), db, prefix, cursor, count)
	if err != nil {
		writeRedisError(w, http.StatusInternalServerError, "failed to read redis tree", err)

		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *apiHandler) handleGetRedisSearch(w http.ResponseWriter, r *http.Request) {
	db, err := parseRedisDB(r.URL.Query().Get("db"))
	if err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid db", err)

		return
	}

	cursor, err := parseRedisCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid cursor", err)

		return
	}

	count, err := parseRedisCount(r.URL.Query().Get("count"))
	if err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid count", err)

		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	resp, err := a.redis.search(r.Context(), db, query, cursor, count)
	if err != nil {
		writeRedisError(w, http.StatusInternalServerError, "redis key search failed", err)

		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *apiHandler) handleGetRedisKey(w http.ResponseWriter, r *http.Request) {
	db, err := parseRedisDB(r.URL.Query().Get("db"))
	if err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid db", err)

		return
	}

	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		writeRedisError(w, http.StatusBadRequest, "key is required", nil)

		return
	}

	resp, err := a.redis.getKey(r.Context(), db, key)
	if err != nil {
		if errors.Is(err, errRedisNotFound) {
			writeRedisError(w, http.StatusNotFound, "key not found", err)

			return
		}

		writeRedisError(w, http.StatusInternalServerError, "failed to read key", err)

		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *apiHandler) handlePostRedisKey(w http.ResponseWriter, r *http.Request) {
	var req redisWriteRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid request body", err)

		return
	}

	if err := validateRedisWriteBody(req); err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid request", err)

		return
	}

	resp, err := a.redis.createKey(r.Context(), req)
	if err != nil {
		var conflict *redisConflictError
		if errors.As(err, &conflict) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "key already exists",
				"code":  "already_exists",
			})

			return
		}

		writeRedisError(w, http.StatusInternalServerError, "failed to create key", err)

		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *apiHandler) handlePutRedisKey(w http.ResponseWriter, r *http.Request) {
	var req redisWriteRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid request body", err)

		return
	}

	if err := validateRedisWriteBody(req); err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid request", err)

		return
	}

	resp, err := a.redis.updateKey(r.Context(), req)
	if err != nil {
		if errors.Is(err, errRedisNotFound) {
			writeRedisError(w, http.StatusNotFound, "key not found", err)

			return
		}

		var conflict *redisConflictError
		if errors.As(err, &conflict) {
			payload := map[string]any{
				"error":          "key changed since it was loaded",
				"code":           "conflict",
				"reloadRequired": true,
			}

			if conflict.Current != nil {
				payload["current"] = conflict.Current
			}

			writeJSON(w, http.StatusConflict, payload)

			return
		}

		writeRedisError(w, http.StatusInternalServerError, "failed to update key", err)

		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *apiHandler) handleDeleteRedisKey(w http.ResponseWriter, r *http.Request) {
	db, err := parseRedisDB(r.URL.Query().Get("db"))
	if err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid db", err)

		return
	}

	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		writeRedisError(w, http.StatusBadRequest, "key is required", nil)

		return
	}

	deleted, err := a.redis.deleteKey(r.Context(), db, key)
	if err != nil {
		writeRedisError(w, http.StatusInternalServerError, "failed to delete key", err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"db":      db,
		"key":     key,
		"deleted": deleted,
	})
}

func (a *apiHandler) handlePostRedisDeleteMany(w http.ResponseWriter, r *http.Request) {
	var req redisDeleteManyRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid request body", err)

		return
	}

	db, err := parseRedisDB(strconv.Itoa(req.DB))
	if err != nil {
		writeRedisError(w, http.StatusBadRequest, "invalid db", err)

		return
	}

	req.DB = db
	if len(req.Keys) == 0 {
		writeRedisError(w, http.StatusBadRequest, "keys must contain at least one key", nil)

		return
	}

	if len(req.Keys) > redisMaxScanItems {
		writeRedisError(w, http.StatusBadRequest, fmt.Sprintf("keys must be <= %d", redisMaxScanItems), nil)

		return
	}

	resp, err := a.redis.deleteKeys(r.Context(), req)
	if err != nil {
		writeRedisError(w, http.StatusInternalServerError, "failed to delete keys", err)

		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseRedisDB(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}

	db, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}

	if db < 0 || db > redisMaxDB {
		return 0, fmt.Errorf("db must be between 0 and %d", redisMaxDB)
	}

	return db, nil
}

func parseRedisCursor(raw string) (uint64, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}

	return strconv.ParseUint(raw, 10, 64)
}

func parseRedisCount(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return redisDefaultScanCount, nil
	}

	count, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}

	if count <= 0 {
		return 0, errors.New("count must be greater than 0")
	}

	if count > redisMaxScanItems {
		return redisMaxScanItems, nil
	}

	return count, nil
}

func validateRedisWriteBody(req redisWriteRequest) error {
	db, err := parseRedisDB(strconv.Itoa(req.DB))
	if err != nil {
		return err
	}

	req.DB = db
	if strings.TrimSpace(req.Key) == "" {
		return errors.New("key is required")
	}

	return nil
}

func writeRedisError(w http.ResponseWriter, status int, message string, err error) {
	payload := map[string]any{
		"error": message,
	}

	if err != nil {
		payload["detail"] = err.Error()
	}

	writeJSON(w, status, payload)
}
