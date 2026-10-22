#!/usr/bin/env python3
"""vaktliste: the duty roster service.

Serves GET /health on the port from the PORT environment variable, which
systemd loads from /etc/vaktliste/vaktliste.env.
"""
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(os.environ["PORT"])
HEALTH = json.dumps({"service": "vaktliste", "status": "ok"}).encode()


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/health":
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(HEALTH + b"\n")

    def log_message(self, fmt, *args):
        pass


ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
