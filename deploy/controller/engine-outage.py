#!/usr/bin/env python3
"""Lab-only Unix socket forwarding proxy; does not stop the shared Docker engine.

SIGUSR1 disables forwarding and closes active streams. SIGUSR2 restores forwarding.
SIGTERM/SIGINT exits and removes only the proxy's own socket.
"""
import argparse
import os
import select
import signal
import socket
import socketserver
import stat
import threading

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--listen', required=True)
parser.add_argument('--target', required=True)
args = parser.parse_args()
if not all(os.path.isabs(p) for p in (args.listen, args.target)):
    parser.error('socket paths must be absolute')
if os.path.lexists(args.listen):
    parser.error('refusing existing proxy path')
parent = os.stat(os.path.dirname(args.listen))
if parent.st_uid != os.getuid() or stat.S_IMODE(parent.st_mode) & 0o077:
    parser.error('proxy socket parent must be private and owned by caller')
lock = threading.Lock()
active = set()
enabled = True


def close_streams():
    with lock:
        streams = list(active)
    for stream in streams:
        try:
            stream.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass
        stream.close()


def disable(_signal, _frame):
    global enabled
    enabled = False
    close_streams()
    print('forwarding disabled; active connections closed', flush=True)


def enable(_signal, _frame):
    global enabled
    enabled = True
    print('forwarding enabled', flush=True)


class Handler(socketserver.BaseRequestHandler):
    def handle(self):
        if not enabled:
            return
        upstream = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        try:
            upstream.settimeout(5)
            upstream.connect(args.target)
            upstream.settimeout(None)
            with lock:
                active.update((self.request, upstream))
            while enabled:
                readable, _, _ = select.select([self.request, upstream], [], [], 1)
                for source in readable:
                    data = source.recv(65536)
                    if not data:
                        return
                    target = upstream if source is self.request else self.request
                    target.sendall(data)
        except (OSError, ValueError):
            pass
        finally:
            with lock:
                active.discard(self.request)
                active.discard(upstream)
            upstream.close()


class Server(socketserver.ThreadingUnixStreamServer):
    daemon_threads = True


os.umask(0o077)
server = Server(args.listen, Handler)
socket_identity = os.stat(args.listen)
signal.signal(signal.SIGUSR1, disable)
signal.signal(signal.SIGUSR2, enable)

def terminate(_signal, _frame):
    raise KeyboardInterrupt

signal.signal(signal.SIGTERM, terminate)
signal.signal(signal.SIGINT, terminate)
print('proxy ready', flush=True)
try:
    server.serve_forever(poll_interval=.1)
except KeyboardInterrupt:
    pass
finally:
    close_streams()
    server.server_close()
    try:
        current = os.stat(args.listen)
        if (current.st_dev, current.st_ino) == (socket_identity.st_dev, socket_identity.st_ino):
            os.unlink(args.listen)
    except FileNotFoundError:
        pass
