package cache

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/cache/persistence"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestCache(t *testing.T) {
	//TODO:unit test
}

func TestWrite(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	store := persistence.NewInMemoryStore(60 * time.Second)
	writer := newCachedWriter(store, time.Second*3, c.Writer, "mykey")
	c.Writer = writer

	c.Writer.WriteHeader(204)
	c.Writer.WriteHeaderNow()
	c.Writer.Write([]byte("foo"))
	assert.Equal(t, 204, c.Writer.Status())
	assert.Equal(t, "foo", w.Body.String())
	assert.True(t, c.Writer.Written())
}

func TestCachePage(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_ping", CachePage(store, time.Second*3, func(c *gin.Context) {
		c.String(200, "pong "+fmt.Sprint(time.Now().UnixNano()))
	}))

	w1 := performRequest("GET", "/cache_ping", router)
	w2 := performRequest("GET", "/cache_ping", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "HIT")
}

func TestCachePageExpire(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_ping", CachePage(store, time.Second, func(c *gin.Context) {
		c.String(200, "pong "+fmt.Sprint(time.Now().UnixNano()))
	}))

	w1 := performRequest("GET", "/cache_ping", router)
	time.Sleep(time.Second * 2)
	w2 := performRequest("GET", "/cache_ping", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.NotEqual(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "MISS")
}

func TestCachePageAtomic(t *testing.T) {
	// memoryDelayStore is a wrapper of a InMemoryStore
	// designed to simulate data race (by doing a delayed write)
	store := newDelayStore(60 * time.Second)

	router := gin.New()
	router.GET("/atomic", CachePageAtomic(store, time.Second*5, func(c *gin.Context) {
		c.String(200, "OK")
	}))

	outp := make(chan string, 10)

	for i := 0; i < 5; i++ {
		go func() {
			resp := performRequest("GET", "/atomic", router)
			outp <- resp.Body.String()
		}()
	}
	time.Sleep(time.Millisecond * 500)
	for i := 0; i < 5; i++ {
		go func() {
			resp := performRequest("GET", "/atomic", router)
			outp <- resp.Body.String()
		}()
	}
	time.Sleep(time.Millisecond * 500)

	for i := 0; i < 10; i++ {
		v := <-outp
		assert.Equal(t, "OK", v)
	}
}

func TestCachePageWithoutHeader(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_ping", CachePageWithoutHeader(store, time.Second*3, func(c *gin.Context) {
		c.String(200, "pong "+fmt.Sprint(time.Now().UnixNano()))
	}))

	w1 := performRequest("GET", "/cache_ping", router)
	w2 := performRequest("GET", "/cache_ping", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.NotNil(t, w1.Header()["Content-Type"])
	assert.Nil(t, w2.Header()["Content-Type"])
	assert.Equal(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "HIT")
}

func TestCachePageWithoutHeaderExpire(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_ping", CachePage(store, time.Second, func(c *gin.Context) {
		c.String(200, "pong "+fmt.Sprint(time.Now().UnixNano()))
	}))

	w1 := performRequest("GET", "/cache_ping", router)
	time.Sleep(time.Second * 2)
	w2 := performRequest("GET", "/cache_ping", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.NotNil(t, w1.Header()["Content-Type"])
	assert.NotNil(t, w2.Header()["Content-Type"])
	assert.NotEqual(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "MISS")
}

func TestCacheHtmlFile(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.LoadHTMLFiles("example/template.html")
	router.GET("/cache_html", CachePage(store, time.Second*3, func(c *gin.Context) {
		c.HTML(http.StatusOK, "template.html", gin.H{"values": fmt.Sprint(time.Now().UnixNano())})
	}))

	w1 := performRequest("GET", "/cache_html", router)
	w2 := performRequest("GET", "/cache_html", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "HIT")
}

func TestCacheHtmlFileExpire(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.LoadHTMLFiles("example/template.html")
	router.GET("/cache_html", CachePage(store, time.Second*1, func(c *gin.Context) {
		c.HTML(http.StatusOK, "template.html", gin.H{"values": fmt.Sprint(time.Now().UnixNano())})
	}))

	w1 := performRequest("GET", "/cache_html", router)
	time.Sleep(time.Second * 2)
	w2 := performRequest("GET", "/cache_html", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.NotEqual(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "MISS")
}

func TestCachePageAborted(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_aborted", CachePage(store, time.Second*3, func(c *gin.Context) {
		c.AbortWithStatusJSON(200, map[string]int64{"time": time.Now().UnixNano()})
	}))

	w1 := performRequest("GET", "/cache_aborted", router)
	time.Sleep(time.Millisecond * 500)
	w2 := performRequest("GET", "/cache_aborted", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.NotEqual(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "MISS")
}

func TestCachePage400(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_400", CachePage(store, time.Second*3, func(c *gin.Context) {
		c.String(400, fmt.Sprint(time.Now().UnixNano()))
	}))

	w1 := performRequest("GET", "/cache_400", router)
	time.Sleep(time.Millisecond * 500)
	w2 := performRequest("GET", "/cache_400", router)

	assert.Equal(t, 400, w1.Code)
	assert.Equal(t, 400, w2.Code)
	assert.NotEqual(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "MISS")
}

func TestCachePageWithoutHeaderAborted(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_aborted", CachePage(store, time.Second*3, func(c *gin.Context) {
		c.AbortWithStatusJSON(200, map[string]int64{"time": time.Now().UnixNano()})
	}))

	w1 := performRequest("GET", "/cache_aborted", router)
	time.Sleep(time.Millisecond * 500)
	w2 := performRequest("GET", "/cache_aborted", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.NotNil(t, w1.Header()["Content-Type"])
	assert.NotNil(t, w2.Header()["Content-Type"])
	assert.NotEqual(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "MISS")
}

func TestCachePageWithoutHeader400(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_400", CachePage(store, time.Second*3, func(c *gin.Context) {
		c.String(400, fmt.Sprint(time.Now().UnixNano()))
	}))

	w1 := performRequest("GET", "/cache_400", router)
	time.Sleep(time.Millisecond * 500)
	w2 := performRequest("GET", "/cache_400", router)

	assert.Equal(t, 400, w1.Code)
	assert.Equal(t, 400, w2.Code)
	assert.NotNil(t, w1.Header()["Content-Type"])
	assert.NotNil(t, w2.Header()["Content-Type"])
	assert.NotEqual(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "MISS")
}

func TestCachePageStatus207(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_207", CachePage(store, time.Second*3, func(c *gin.Context) {
		c.String(207, fmt.Sprint(time.Now().UnixNano()))
	}))

	w1 := performRequest("GET", "/cache_207", router)
	time.Sleep(time.Millisecond * 500)
	w2 := performRequest("GET", "/cache_207", router)

	assert.Equal(t, 207, w1.Code)
	assert.Equal(t, 207, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "HIT")
}

func TestCachePageWithoutQuery(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_without_query", CachePageWithoutQuery(store, time.Second*3, func(c *gin.Context) {
		c.String(200, "pong "+fmt.Sprint(time.Now().UnixNano()))
	}))

	w1 := performRequest("GET", "/cache_without_query?foo=1", router)
	w2 := performRequest("GET", "/cache_without_query?foo=2", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "HIT")
}

func TestRegisterResponseCacheGob(t *testing.T) {
	RegisterResponseCacheGob()
	r := responseCache{Status: 200, Data: []byte("test")}
	mCache := new(bytes.Buffer)
	encCache := gob.NewEncoder(mCache)
	err := encCache.Encode(r)
	assert.Nil(t, err)

	var decodedResp responseCache
	pCache := bytes.NewBuffer(mCache.Bytes())
	decCache := gob.NewDecoder(pCache)
	err = decCache.Decode(&decodedResp)
	assert.Nil(t, err)

}

func TestCacheDisallowedHeadersAddedInHandler(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_ping", CachePage(store, time.Second*3, func(c *gin.Context) {
		// Set the response headers in the handler
		c.Header("Authorization", "BAD CACHE, no biscuit!")
		c.Header("X-Test-Header", "Good Boy")
		c.String(200, "pong "+fmt.Sprint(time.Now().UnixNano()))
	}))

	w1 := performRequest("GET", "/cache_ping", router)
	w2 := performRequest("GET", "/cache_ping", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Test-Header"), "Good Boy")
	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	// The uncached version should return the Authorization header
	assert.NotEqual(t, w1.Header().Get("Authorization"), "")

	assert.Equal(t, w2.Header().Get("X-Test-Header"), "Good Boy")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "HIT")
	// The cached version should not return the Authorization header
	assert.Equal(t, w2.Header().Get("Authorization"), "")
}

func TestCacheDisallowedHeadersAddedBeforeCache(t *testing.T) {
	store := persistence.NewInMemoryStore(60 * time.Second)

	router := gin.New()
	router.GET("/cache_ping", func(c *gin.Context) {
		// Record the exact time that the Authorization header was created
		now := time.Now().UnixNano()
		c.Set("MM-TimeNow", now)

		// Set the response headers before the cache
		c.Header("Authorization", "pong "+fmt.Sprint(now))
		c.Header("X-Test-Header", "Good Boy")
		c.Next()
	}, CachePage(store, time.Second*3, func(c *gin.Context) {
		// Use exact time that the Authorization header was created in the body
		c.String(200, "pong "+fmt.Sprint(c.Value("MM-TimeNow")))
	}))

	w1 := performRequest("GET", "/cache_ping", router)
	w2 := performRequest("GET", "/cache_ping", router)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String())

	assert.Equal(t, w1.Header().Get("X-Test-Header"), "Good Boy")
	assert.Equal(t, w1.Header().Get("X-Cache-Status"), "MISS")
	// The uncached version should return the Authorization header
	assert.NotEqual(t, w1.Header().Get("Authorization"), "")
	// The header should match the body
	assert.Equal(t, w1.Header().Get("Authorization"), w1.Body.String())

	assert.Equal(t, w2.Header().Get("X-Test-Header"), "Good Boy")
	assert.Equal(t, w2.Header().Get("X-Cache-Status"), "HIT")
	// The cached version should return the Authorization header
	assert.NotEqual(t, w2.Header().Get("Authorization"), "")
	// The header should NOT have been cached (and so not match the body)
	assert.NotEqual(t, w2.Header().Get("Authorization"), w2.Body.String())
}

func performRequest(method, target string, router *gin.Engine) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

type memoryDelayStore struct {
	*persistence.InMemoryStore
}

func newDelayStore(defaultExpiration time.Duration) *memoryDelayStore {
	v := &memoryDelayStore{}
	v.InMemoryStore = persistence.NewInMemoryStore(defaultExpiration)
	return v
}

func (c *memoryDelayStore) Set(key string, value interface{}, expires time.Duration) error {
	time.Sleep(time.Millisecond * 3)
	return c.InMemoryStore.Set(key, value, expires)
}

func (c *memoryDelayStore) Add(key string, value interface{}, expires time.Duration) error {
	time.Sleep(time.Millisecond * 3)
	return c.InMemoryStore.Add(key, value, expires)
}
