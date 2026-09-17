package pwakit_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	pwakit "github.com/kilo666mj/pwa-kit"
)

func ExampleHandler() {
	request := httptest.NewRequest(http.MethodGet, "https://app.example.com/pwa-kit/browser.js", nil)
	response := httptest.NewRecorder()
	pwakit.Handler().ServeHTTP(response, request)

	fmt.Println(response.Code, response.Header().Get("Content-Type"))
	// Output: 200 text/javascript; charset=utf-8
}
