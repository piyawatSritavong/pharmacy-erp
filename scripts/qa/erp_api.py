"""Small client for driving the ERP API from QA and demo scripts.

One cookie session per account, thread-safe, standard library only.

Environment:
  ERP_BASE            API base, default http://localhost:8090/api/v1
  DATABASE_URL        database the API writes to (read-only checks via psql)
  SEED_ADMIN_PASSWORD / SEED_POS_PASSWORD
                      the seeded accounts' passwords. When unset, the local
                      development values are read from deploy/docker-compose.yml.
                      Never point these scripts at production.
"""
import http.cookiejar
import json
import os
import re
import subprocess
import threading
import urllib.error
import urllib.parse
import urllib.request

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
BASE = os.environ.get("ERP_BASE", "http://localhost:8090/api/v1")
DATABASE_URL = os.environ.get("DATABASE_URL", "postgres://pharmacy:pharmacy@localhost:5442/pharmacy_demo?sslmode=disable")

ACCOUNTS = {
    "super": "superadmin@erp.local",
    "central": "admin.central@erp.local",
    "MES": "pos.mes@erp.local",
    "PHH": "pos.phahol@erp.local",
    "PHS": "pos.phasuk@erp.local",
    "NPT": "pos.nakhonpathom@erp.local",
    "KNP": "pos.knp@erp.local",
}


def _password(kind):
    value = os.environ.get(kind)
    if value:
        return value
    compose = open(os.path.join(ROOT, "deploy", "docker-compose.yml"), encoding="utf-8").read()
    return re.search(r'%s: "([^"]+)"' % kind, compose).group(1)


class Response:
    def __init__(self, status, body, headers):
        self.status, self.body, self.headers = status, body, headers

    def ok(self):
        return 200 <= self.status < 300

    def __repr__(self):
        text = self.body if isinstance(self.body, (dict, list, str)) else "<%d bytes>" % len(self.body or b"")
        return "<%s %s>" % (self.status, json.dumps(text, ensure_ascii=False, default=str)[:500])


_sessions = {}
_lock = threading.Lock()


def _send(opener, method, path, body=None, query=None, timeout=120):
    url = BASE + path
    if query:
        url += ("&" if "?" in url else "?") + urllib.parse.urlencode(query, doseq=True)
    data, headers = None, {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=data, method=method, headers=headers)
    try:
        with opener.open(request, timeout=timeout) as resp:
            payload = resp.read()
            ctype = resp.headers.get("content-type", "")
            parsed = json.loads(payload) if "json" in ctype and payload else payload
            return Response(resp.status, parsed, dict(resp.headers))
    except urllib.error.HTTPError as error:
        payload = error.read()
        try:
            parsed = json.loads(payload)
        except Exception:
            parsed = payload.decode(errors="replace")
        return Response(error.code, parsed, dict(error.headers))


def session(account):
    with _lock:
        if account in _sessions:
            return _sessions[account]
    email = ACCOUNTS.get(account, account)
    password = _password("SEED_POS_PASSWORD" if email.startswith("pos.") else "SEED_ADMIN_PASSWORD")
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    resp = _send(opener, "POST", "/auth/login", {"email": email, "password": password})
    if resp.status != 200:
        raise RuntimeError("login %s failed with %s" % (account, resp.status))
    with _lock:
        _sessions[account] = (opener, resp.body)
    return _sessions[account]


def me(account):
    return session(account)[1]


def api(account, method, path, body=None, query=None):
    return _send(session(account)[0], method, path, body, query)


def sql(statement):
    """Runs a read-only statement and returns rows as lists of strings."""
    out = subprocess.run(["psql", DATABASE_URL, "-At", "-F", "\t", "-v", "ON_ERROR_STOP=1", "-c", statement],
                         capture_output=True, text=True)
    if out.returncode != 0:
        raise RuntimeError(out.stderr.strip())
    return [line.split("\t") for line in out.stdout.splitlines() if line != ""]


def sql_value(statement):
    rows = sql(statement)
    return rows[0][0] if rows else None
