package cli

import (
	"errors"

	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/runtime"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/locales"
)

// userError is a failure the learner can act on. key names a locale string
// and vars fill it. cause, if set, is the technical detail, shown after the
// translated message.
type userError struct {
	key   string
	vars  map[string]string
	cause error
}

func (e *userError) Error() string {
	if e.cause != nil {
		return e.key + ": " + e.cause.Error()
	}
	return e.key
}

func (e *userError) Unwrap() error { return e.cause }

// fail returns a userError for key, with an optional technical cause.
func fail(key string, cause error) error { return &userError{key: key, cause: cause} }

// failWith is fail with values for the key's placeholders.
func failWith(key string, vars map[string]string, cause error) error {
	return &userError{key: key, vars: vars, cause: cause}
}

// Message renders err for the learner in lang (spec section 12). Known
// failures get their own text. Anything else gets a translated heading
// followed by its technical detail; that detail comes from Go, Docker or
// Postgres and is not translated.
func Message(b *i18n.Bundle, lang string, err error) string {
	var ue *userError
	switch {
	case errors.As(err, &ue):
		msg := b.T(lang, ue.key, ue.vars)
		if ue.cause != nil {
			msg += "\n" + b.T(lang, "error.detail", map[string]string{"detail": ue.cause.Error()})
		}
		return msg
	case errors.Is(err, session.ErrGiveUpTooEarly):
		return b.T(lang, "giveup.locked", nil)
	case errors.Is(err, session.ErrNoShell):
		return b.T(lang, "error.no_shell", nil)
	case errors.Is(err, runtime.ErrNeedsCgroupV2):
		return b.T(lang, "error.cgroup_v2_required", nil)
	case errors.Is(err, runtime.ErrSystemdUnsupported):
		return b.T(lang, "error.systemd_unsupported", nil)
	default:
		return b.T(lang, "error.generic", map[string]string{"detail": err.Error()})
	}
}

// English renders an error in English, for the developer commands, which run
// without a learner profile.
func English(err error) string {
	b, lerr := i18n.Load(locales.FS)
	if lerr != nil {
		return err.Error()
	}
	return Message(b, "en", err)
}
