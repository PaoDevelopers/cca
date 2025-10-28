# YK Pao School Co-curricular Activities Selections System

[Main repo](https://git.sr.ht/~runxiyu/cca)\
[Issue tracker](https://todo.sr.ht/~runxiyu/cca)

## Build

You need a recent [Go](https://go.dev) toolchain. [`sqlc`](https://sqlc.dev) is
necessary but will be downloaded and run automatically if absent from `$PATH`.

To build, just run `./build`.

## Configuration and setup

Adapt `cca.scfgs` to your environment.

Note that this service does not have automatic database schema migrations.
Instance administrators are required to run the schema and relevant migrations
themselves.

### Disable nginx proxying

Reverse proxies such as nginx may buffer responses by default.

In addition to typical reverse proxy rules, add separate rules for the SSE endpoint:

```
location /student/api/events {
	proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
	proxy_set_header Host $host;
	proxy_pass http://127.0.0.1:8080;
	proxy_http_version 1.1;
	proxy_set_header Connection '';

	proxy_buffering off;
	proxy_cache off;
	proxy_read_timeout 3600s;
	proxy_send_timeout 3600s;
	chunked_transfer_encoding off;
}
```
