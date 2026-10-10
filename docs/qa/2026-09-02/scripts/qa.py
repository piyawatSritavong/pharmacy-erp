#!/usr/bin/env python3
"""QA helper: login per role, call API, record test results."""
import json, sys, os, time, urllib.request, urllib.error, urllib.parse, datetime
BASE = os.environ.get("QA_BASE", "http://localhost:8080/api/v1")
PW = "DevPassword123!"
ACCOUNTS = {
  "super": "superadmin@erp.local",
  "central": "admin.central@erp.local",
  "admin.mes": "admin.mes@erp.local",
  "pos.mes": "pos.mes@erp.local",
  "pos.phh": "pos.phahol@erp.local",
  "pos.phs": "pos.phasuk@erp.local",
  "pos.npt": "pos.nakhonpathom@erp.local",
  "pos.knp": "pos.knp@erp.local",
  "pos.wh": "pos.warehouse@erp.local",
}
_tokens = {}
_users = {}
LOG = os.path.join(os.path.dirname(os.path.abspath(__file__)), "results.jsonl")

def login(role):
    if role in _tokens: return _tokens[role]
    email = ACCOUNTS.get(role, role)
    r = raw("POST", "/auth/login", {"email": email, "password": PW}, token=None)
    if r["status"] != 200:
        raise RuntimeError(f"login {role} failed: {r}")
    _tokens[role] = r["body"]["token"]
    _users[role] = r["body"]
    return _tokens[role]

def me(role):
    login(role); return _users[role]

def raw(method, path, body=None, token=None, query=None, raw_body=None, headers=None):
    url = BASE + path
    if query:

        url += ("&" if "?" in url else "?") + urllib.parse.urlencode(query, doseq=True)
    data = None
    hdrs = {"Accept": "application/json"}
    if headers: hdrs.update(headers)
    if raw_body is not None:
        data = raw_body
    elif body is not None:
        data = json.dumps(body).encode(); hdrs["Content-Type"] = "application/json"
    if token: hdrs["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=data, method=method, headers=hdrs)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            ct = resp.headers.get("content-type", "")
            payload = resp.read()
            try: b = json.loads(payload) if "json" in ct else payload
            except Exception: b = payload
            return {"status": resp.status, "body": b, "headers": dict(resp.headers)}
    except urllib.error.HTTPError as e:
        payload = e.read()
        try: b = json.loads(payload)
        except Exception: b = payload.decode(errors="replace")
        return {"status": e.code, "body": b, "headers": dict(e.headers)}

def api(role, method, path, body=None, query=None, **kw):
    return raw(method, path, body, token=login(role) if role else None, query=query, **kw)

def record(tc, area, role, title, status, expected="", actual="", notes=""):
    """status: PASS | FAIL | BLOCKED | UNTESTABLE | INFO"""
    entry = {"ts": datetime.datetime.now().isoformat(timespec="seconds"), "tc": tc, "area": area, "role": role,
             "title": title, "status": status, "expected": expected, "actual": actual, "notes": notes}
    with open(LOG, "a") as f: f.write(json.dumps(entry, ensure_ascii=False) + "\n")
    print(f"[{status}] {tc} {title} :: {actual[:160] if isinstance(actual,str) else actual}")

def pp(x): print(json.dumps(x, ensure_ascii=False, indent=1, default=str)[:4000])

if __name__ == "__main__":
    role, method, path = sys.argv[1], sys.argv[2], sys.argv[3]
    body = json.loads(sys.argv[4]) if len(sys.argv) > 4 else None
    pp(api(role, method, path, body))
