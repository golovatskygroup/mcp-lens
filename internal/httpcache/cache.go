package httpcache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type cacheEntry struct {
	key      string
	status   int
	header   http.Header
	body     []byte
	etag     string
	storedAt time.Time
}

type Cache struct {
	ttl        time.Duration
	maxEntries int

	mu      sync.Mutex
	entries map[string]*list.Element
	lru     *list.List // Front = most recent
}

func New(ttl time.Duration, maxEntries int) *Cache {
	if maxEntries <= 0 {
		maxEntries = 512
	}
	if ttl < 0 {
		ttl = 0
	}
	return &Cache{
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    map[string]*list.Element{},
		lru:        list.New(),
	}
}

func (c *Cache) Get(key string) (cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		return cacheEntry{}, false
	}
	c.lru.MoveToFront(el)
	return el.Value.(cacheEntry), true
}

func (c *Cache) Put(key string, status int, header http.Header, body []byte, storedAt time.Time) cacheEntry {
	ent := cacheEntry{
		key:      key,
		status:   status,
		header:   cloneHeader(header),
		body:     append([]byte(nil), body...),
		etag:     strings.TrimSpace(header.Get("ETag")),
		storedAt: storedAt,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		el.Value = ent
		c.lru.MoveToFront(el)
		return ent
	}
	el := c.lru.PushFront(ent)
	c.entries[key] = el

	for c.maxEntries > 0 && c.lru.Len() > c.maxEntries {
		back := c.lru.Back()
		if back == nil {
			break
		}
		be := back.Value.(cacheEntry)
		delete(c.entries, be.key)
		c.lru.Remove(back)
	}
	return ent
}

func (c *Cache) Touch(key string, storedAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		ent := el.Value.(cacheEntry)
		ent.storedAt = storedAt
		el.Value = ent
		c.lru.MoveToFront(el)
	}
}

func (c *Cache) TTL() time.Duration { return c.ttl }

func cloneHeader(h http.Header) http.Header {
	out := http.Header{}
	for k, vv := range h {
		cp := make([]string, len(vv))
		copy(cp, vv)
		out[k] = cp
	}
	return out
}

func fingerprintHeaders(h http.Header, keys []string) string {
	type kv struct {
		k string
		v string
	}
	pairs := make([]kv, 0, len(keys))
	for _, k := range keys {
		k = http.CanonicalHeaderKey(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		v := strings.TrimSpace(h.Get(k))
		if v == "" {
			continue
		}
		pairs = append(pairs, kv{k: k, v: v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })

	sum := sha256.New()
	for _, p := range pairs {
		sum.Write([]byte(p.k))
		sum.Write([]byte{0})
		sum.Write([]byte(p.v))
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil))
}
