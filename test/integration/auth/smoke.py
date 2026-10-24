"""Local HTTP/Redis/MailHog smoke. Creates one unique test user and reset email."""
import json
import os
import re
import time
import urllib.error
import urllib.request
import uuid

API = os.environ.get('TEST_API_URL', 'http://localhost:8080')
MAILHOG = os.environ.get('TEST_MAILHOG_URL', 'http://localhost:8025')


def request(path, body=None, token=None, expected=200):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request(API + path,
                                 data=json.dumps(body).encode() if body is not None else None,
                                 headers=headers)
    try:
        response = urllib.request.urlopen(req, timeout=8)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read()
        assert response.status == expected, (path, response.status, expected)
        result = json.loads(raw) if raw else None
        if expected >= 400:
            assert {'code', 'message', 'requestId'} <= set(result) <= {'code', 'message', 'requestId', 'field'}, result
        return result


email = 'smoke-' + uuid.uuid4().hex + '@example.com'
request('/readyz')
first = request('/v1/auth/register', {'email': email, 'password': 'initial-password', 'name': 'Smoke'}, expected=201)
user = request('/v1/users/me', token=first['accessToken'])
assert user['email'] == email and isinstance(user['id'], str)
request('/v1/auth/login', {'email': email, 'password': 'wrong-password'}, expected=401)
second = request('/v1/auth/refresh', {'refreshToken': first['refreshToken']})
request('/v1/auth/refresh', {'refreshToken': first['refreshToken']}, expected=401)
request('/v1/users/me', token=first['accessToken'], expected=401)
request('/v1/auth/forgot-password', {'email': email}, expected=202)
reset_token = None
for _ in range(20):
    with urllib.request.urlopen(MAILHOG + '/api/v2/messages?limit=100', timeout=3) as response:
        messages = json.load(response)['items']
    for message in messages:
        if email in message['Content']['Headers'].get('To', []):
            match = re.search(r'#token=([A-Za-z0-9_-]+)', message['Content']['Body'])
            if match:
                reset_token = match.group(1)
    if reset_token:
        break
    time.sleep(0.1)
assert reset_token, 'Reset email not found in local MailHog'
request('/v1/auth/reset-password', {'token': reset_token, 'password': 'new-password'}, expected=204)
request('/v1/auth/reset-password', {'token': reset_token, 'password': 'new-password'}, expected=401)
request('/v1/users/me', token=second['accessToken'], expected=401)
request('/v1/auth/refresh', {'refreshToken': second['refreshToken']}, expected=401)
request('/v1/auth/login', {'email': email, 'password': 'initial-password'}, expected=401)
third = request('/v1/auth/login', {'email': email, 'password': 'new-password'})
request('/v1/auth/logout', {'refreshToken': third['refreshToken']}, expected=204)
request('/v1/users/me', token=third['accessToken'], expected=401)
print('PASS: register, login, profile, refresh replay, MailHog delivery, reset replay, session revocation, logout')
