package sqlite

import "time"

// utc normalizes a time before it is bound to a statement. Every time.Time written
// to this database must go through it.
//
// The modernc driver stores time.Time as a string that carries its zone offset, and
// SQLite compares DATETIME values as strings. A row written as "+09:00" therefore
// sorts and compares incorrectly against one written as "Z", which silently breaks
// range queries — we already shipped that bug once in the review reminder scheduler
// (#18), where a due-window comparison stopped matching depending on the writer's
// zone. Normalizing at the single point where a value crosses into SQL is the only
// place that guarantees it, because the callers are spread across every repository.
//
// Date boundaries ("today", "this week") must also be computed in Go and passed as
// explicit UTC instants rather than expressed with SQLite's date() functions, so
// that both sides of a comparison are produced the same way.
func utc(t time.Time) time.Time {
	return t.UTC()
}
