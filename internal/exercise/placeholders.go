package exercise

import "github.com/DIZAMU/RootCause/internal/i18n"

// UnfilledPlaceholders returns the {name} tokens in a language file that are
// not in available, sorted. It looks at the ticket, every hint, the
// walkthrough and every feedback text. available holds the names a session
// can fill: the variant names and the derived values break.sh prints.
func UnfilledPlaceholders(t *I18n, available map[string]bool) []string {
	texts := []string{t.Ticket.Title, t.Ticket.From, t.Ticket.Body, t.Walkthrough}
	texts = append(texts, t.Hints...)
	for _, v := range t.Feedback {
		texts = append(texts, v)
	}
	missing := make(map[string]bool)
	for _, s := range texts {
		for _, name := range i18n.Placeholders(s) {
			if !available[name] {
				missing[name] = true
			}
		}
	}
	return sortedKeys(missing)
}
