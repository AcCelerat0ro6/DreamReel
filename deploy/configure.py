#!/usr/bin/env python3
"""Create server-only credentials once; never print or replace existing secrets."""
import os
from pathlib import Path
import pwd
import secrets
import subprocess
import sys

import yaml


def main():
    root = Path(sys.argv[1]).resolve()
    owner = pwd.getpwnam(sys.argv[2])
    runtime = root / '.runtime'
    runtime.mkdir(mode=0o750, exist_ok=True)
    config_path = runtime / 'config.yaml'
    if config_path.exists():
        cfg = yaml.safe_load(config_path.read_text(encoding='utf-8'))
        if 'host' not in cfg:
            cfg['host'] = '127.0.0.1'
            # Write atomically, keeping all existing credentials and settings.
            temporary = runtime / 'config.yaml.next'
            fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            with os.fdopen(fd, 'w', encoding='utf-8') as output:
                yaml.safe_dump(cfg, output, allow_unicode=True, sort_keys=False)
            os.replace(temporary, config_path)
        os.chmod(config_path, 0o600)
        os.chown(config_path, owner.pw_uid, owner.pw_gid)
        print('Keeping existing server configuration.')
        return
    bootstrap = runtime / 'bootstrap.yaml'
    if not bootstrap.exists():
        raise SystemExit('First deployment needs .runtime/bootstrap.yaml with target MySQL admin, Redis and RabbitMQ settings. See docs/deployment.md.')
    cfg = yaml.safe_load(bootstrap.read_text(encoding='utf-8-sig'))
    db = cfg['database']
    # Password travels through the environment, rather than command-line arguments.
    env = dict(os.environ, MYSQL_PWD=str(db['password']))
    database = str(db['name'])
    if not database.replace('_', '').isalnum() or not database.isascii():
        raise SystemExit('Database name must contain ASCII letters, numbers or underscores.')
    app_user = 'dreamreel_app'
    app_password = secrets.token_hex(24)
    sql = (
        f'CREATE DATABASE IF NOT EXISTS `{database}` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n'
        f"CREATE USER '{app_user}'@'127.0.0.1' IDENTIFIED BY '{app_password}';\n"
        f"GRANT ALL PRIVILEGES ON `{database}`.* TO '{app_user}'@'127.0.0.1';\n"
    )
    result = subprocess.run(
        ['mysql', '-h', str(db['host']), '-P', str(db['port']), '-u', str(db['user'])],
        input=sql, text=True, env=env, capture_output=True,
    )
    if result.returncode:
        # Avoid logging an SQL statement containing the generated password.
        raise SystemExit('MySQL provisioning failed. Check administrator credentials and whether dreamreel_app already exists. No config was overwritten.')
    cfg['host'] = '127.0.0.1'
    cfg['port'] = 8080
    cfg['mode'] = 'release'
    cfg['jwt']['secret'] = secrets.token_hex(32)
    cfg['internal']['token'] = secrets.token_hex(32)
    db.update(host='127.0.0.1', user=app_user, password=app_password)
    # Use a dedicated Redis logical DB to avoid local-development cache collisions.
    cfg['redis'].update(addr='127.0.0.1:6379', db=1)
    cfg['log'].update(level='info', filename=str(runtime / 'logs' / 'DreamReel.log'))
    fd = os.open(config_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w', encoding='utf-8') as output:
        yaml.safe_dump(cfg, output, allow_unicode=True, sort_keys=False)
    os.chown(config_path, owner.pw_uid, owner.pw_gid)
    bootstrap.unlink()
    print('Created server configuration and a dedicated application database user.')


if __name__ == '__main__':
    main()
