package sqlite

// learnerIsActive is the single definition of "this item is still being learned".
//
// It is a string constant rather than a bound parameter because the same rule has to
// hold in four independent queries — due cards, the due-card count behind the review
// reminder, the dashboard, and grading — and when they disagree the app contradicts
// itself: a notification says three cards are due and the review screen shows none.
// Keeping the predicate in one place makes them impossible to drift apart.
//
// COALESCE covers cards whose knowledge item has no learner row at all, which is a
// real state during the window between a card being created and its learner row being
// read back; those count as active.
const learnerIsActive = `COALESCE(li.status, 'active') NOT IN ('known', 'removed')`
