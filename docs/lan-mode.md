## Backups (SQLite + files)

    Backup mindengage.db and ./data/ regularly (cron or a small script).

    Quick script (Linux):
```
#!/usr/bin/env bash
ts=$(date +%Y%m%d-%H%M%S)
mkdir -p backups
sqlite3 mindengage.db ".backup 'backups/mindengage-$ts.db'"
tar -czf backups/assets-$ts.tgz data/
```


## Offline user management

    Keep your current /auth/login (username=password) for development only.

    For production on LAN, consider a tiny users.json loaded at boot (hashed passwords) or a local users table (SQLite). You can still mint the same JWTs.

