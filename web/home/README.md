# MindEngage Home

Landing page for MindEngage LMS, served at site root `/` by the Go gateway. Links to **/exam/**, **/teacher/**, **/admin/**.

**Dev:** `npm install && npm start` ([http://localhost:3000](http://localhost:3000))
**Build (root path):** `PUBLIC_URL=/ npm run build`
**Copy to gateway (embed):** `mkdir -p ../../cmd/gateway/static/home && rm -rf ../../cmd/gateway/static/home/* && cp -a build/* ../../cmd/gateway/static/home/`
**Run gateway:** `cd ../../cmd/gateway && go build -o gateway && ./gateway` ([http://localhost:8080](http://localhost:8080))

**Other apps (so nav works):**

* Exam → `PUBLIC_URL=/exam npm run build` → copy to `cmd/gateway/static/exam/`
* Teacher → `REACT_APP_API_BASE=/api PUBLIC_URL=/teacher npm run build` → `cmd/gateway/static/teacher/`
* Admin → `PUBLIC_URL=/admin npm run build` → `cmd/gateway/static/admin/`

**Troubleshoot:**

* 404 `/static/js/*` → rebuild with `PUBLIC_URL=/`
* Old UI → recopy `build/*` → `static/home/` and rebuild gateway
