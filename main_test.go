package main

import (
	"net/http"
	"testing"
	//
	"github.com/labstack/echo"
	"github.com/labstack/echo/engine/standard"
	//"github.com/labstack/echo/test"
	//"github.com/stretchr/testify/assert"
	//	"net/http/httptest"
	//"strings"
	"net/http/httptest"
)

//
//func request(method, path string, e *echo.Echo) (int, string) {
//	req := test.NewRequest(method, path, nil)
//	rec := test.NewResponseRecorder()
//	e.ServeHTTP(req, rec)
//	return rec.Status(), rec.Body.String()
//}

func TestGetServices(t *testing.T) {
	//assert := assert.New(t)

	e := echo.New()
	req := new(http.Request)
	rec := httptest.NewRecorder()
	c := e.NewContext(standard.NewRequest(req, e.Logger()), standard.NewResponse(rec, e.Logger()))

	//c := e.NewContext(standard.NewRequest(req), standard.NewResponse(rec))

	if err := ListServices(c); err != nil {
		t.Error(err)
	}

	//assert.NotEqual(1, 0, "should match")

	// Setup
	//e := echo.New()

	//req, err := http.NewRequest(echo.GET, "/services")
	//assert.NoError(err)
	//assert
	//
	//

	//r := new(http.Request)
	//rec := httptest.NewRecorder()
	//c := e.NewContext(standard.NewRequest(r, e.Logger()), standard.NewResponse(rec, e.Logger()))

	//c, b := request(echo.GET, "/images/walle.png", e)
	//assert.Equal(t, http.StatusOK, c)
	//assert.NotEmpty(t, b)
	//
	//assert.Equal(e.Routes(), "foo", "should match")

	//c.SetPath("/services")
	//h := ListServices(c)
	//assert.Equal(h, http.StatusOK, "Should match")

	//c.SetParamValues("jon@labstack.com")
	//h := &handler{mockDB}
	//
	//// Assertions
	//if assert.NoError(t, h.getUser(c)) {
	//	assert.Equal(t, http.StatusOK, rec.Code)
	//	assert.Equal(t, "foo:bar", rec.Body.String())
	//}
}
