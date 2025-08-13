package sqlstore_test

import (
	_ "github.com/lib/pq"  // registers "postgres" (used in PG tests)
	_ "modernc.org/sqlite" // registers driver name "sqlite"
)
