package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

// distFS embeds the built React client (client/dist, copied here as
// internal/httpapi/dist before `go build` — see README) so that the server
// binary alone can serve both the API and the UI. Until a real client build
// is copied in, dist contains only .gitkeep, which keeps `go:embed` (it
// requires at least one matched file) and `go build` working out of the box.
//
//go:embed all:dist
var distFS embed.FS

func staticFS() http.FileSystem {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}

// spaHandler serves the embedded client build, falling back to index.html
// for any path that isn't a real static file — so client-side routes (e.g.
// /admin/users) resolve correctly on a full page load or refresh.
func spaHandler(fsys http.FileSystem) http.HandlerFunc {
	fileServer := http.FileServer(fsys)
	return func(w http.ResponseWriter, r *http.Request) {
		if f, err := fsys.Open(r.URL.Path); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	}
}
