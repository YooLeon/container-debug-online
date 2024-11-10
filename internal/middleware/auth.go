package middleware

import (
	"crypto/subtle"
	"net/http"
)

// 定义需要密码保护的路径
var protectedPaths = map[string]bool{
	"/containers": true,
	"/ws":         true,
	// 可以添加更多需要保护的路径
}

// AuthMiddleware 创建认证中间件
func AuthMiddleware(password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 如果没有设置密码或路径不需要保护，直接放行
			if password == "" || !protectedPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// 获取Basic Auth信息
			user, pass, ok := r.BasicAuth()
			if !ok || user != "admin" || subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
