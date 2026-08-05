-- The inbox screen became the search history, and a notification's route is the
-- screen name itself. The name is written into this table when the notification is
-- created, so rows enqueued before the rename still say "Inbox" — and the frontend
-- stops understanding that word the moment its compatibility map is deleted.
--
-- Rewriting them here is what lets that map go: after this migration no living row
-- carries the old name, so "Search History" is the only spelling any language has to
-- know. Acked and expired rows are rewritten too — the app-internal notification
-- history still lets the user click a months-old reminder.
UPDATE notifications SET route = 'Search History' WHERE route = 'Inbox';
