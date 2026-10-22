#!/usr/bin/env python3
"""lagerapp: the warehouse stock service.

Reads /etc/lagerapp/lagerapp.ini, counts the stock files in its data
directory and serves GET /health on 127.0.0.1. Every start and every
configuration error is written to /var/log/lagerapp/lagerapp.log.
"""
import configparser
import json
import logging
import os
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

CONFIG = "/etc/lagerapp/lagerapp.ini"
LOG = "/var/log/lagerapp/lagerapp.log"

logging.basicConfig(
    filename=LOG,
    level=logging.INFO,
    format="%(asctime)s lagerapp[%(process)d] %(levelname)s %(message)s",
)
log = logging.getLogger()


def fatal(msg):
    log.error(msg)
    log.error("startup failed, exiting")
    sys.exit(1)


def load_config():
    cfg = configparser.ConfigParser()
    if not cfg.read(CONFIG):
        fatal(f"cannot read config file {CONFIG}")
    settings = {}
    for section, key in (("server", "port"), ("storage", "data_dir")):
        if not cfg.has_option(section, key):
            fatal(f"missing setting {key} in section [{section}] of {CONFIG}")
        settings[key] = cfg.get(section, key).strip()
    try:
        settings["port"] = int(settings["port"])
    except ValueError:
        fatal(f"setting port in section [server] is not a number: {settings['port']!r}")
    if not os.path.isdir(settings["data_dir"]):
        fatal(f"data_dir {settings['data_dir']} does not exist (setting data_dir in section [storage])")
    return settings


def main():
    log.info("starting, reading %s", CONFIG)
    settings = load_config()
    records = len([f for f in os.listdir(settings["data_dir"]) if f.endswith(".csv")])
    health = json.dumps(
        {"status": "ok", "port": settings["port"], "data_dir": settings["data_dir"], "records": records},
        sort_keys=True,
    ).encode()

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path != "/health":
                self.send_error(404)
                return
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(health + b"\n")

        def log_message(self, fmt, *args):
            log.info("request %s", fmt % args)

    try:
        server = ThreadingHTTPServer(("127.0.0.1", settings["port"]), Handler)
    except OSError as e:
        fatal(f"cannot listen on port {settings['port']}: {e}")
    log.info("loaded %d stock files from %s, listening on 127.0.0.1:%d", records, settings["data_dir"], settings["port"])
    server.serve_forever()


if __name__ == "__main__":
    main()
