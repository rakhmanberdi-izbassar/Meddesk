#!/usr/bin/env python3
"""Run bootstrap tests against a disposable PostgreSQL container; never production."""
import os, secrets, subprocess, time, uuid
from pathlib import Path
root=Path(__file__).resolve().parents[1]
name='meddesk-bootstrap-test-'+uuid.uuid4().hex[:10]
password=secrets.token_hex(24)
def command(args,**kwargs):
    return subprocess.check_output(args,stderr=subprocess.STDOUT,text=True,**kwargs).strip()
try:
    command(['docker','run','-d','--name',name,'-e','POSTGRES_USER=meddesk_test','-e','POSTGRES_DB=meddesk_test','-e','POSTGRES_PASSWORD='+password,'-p','127.0.0.1::5432','postgres:18-alpine'])
    for attempt in range(60):
        check=subprocess.run(['docker','exec',name,'pg_isready','-h','127.0.0.1','-U','meddesk_test','-d','meddesk_test'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        if check.returncode==0: break
        time.sleep(.5)
    else: raise RuntimeError('Disposable database not ready')
    port=command(['docker','port',name,'5432/tcp']).rsplit(':',1)[1]
    internal='postgres://meddesk_test:'+password+'@127.0.0.1:5432/meddesk_test?sslmode=disable'
    print(command(['docker','run','--rm','--network','container:'+name,'-v',str(root/'backend/db/migrations')+':/migrations:ro','migrate/migrate:v4.19.1','-path=/migrations','-database='+internal,'up']),flush=True)
    env=dict(os.environ,TEST_DATABASE_URL='postgres://meddesk_test:'+password+'@127.0.0.1:'+port+'/meddesk_test?sslmode=disable')
    subprocess.run(['go','test','-race','-count=1','-v','./tests','-run','^TestBootstrap'],cwd=root/'backend',env=env,check=True)
    print('PASS isolated PostgreSQL bootstrap checks',flush=True)
except subprocess.CalledProcessError:
    print('Disposable bootstrap check failed; connection strings were withheld',flush=True)
    raise SystemExit(1)
finally:
    subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
