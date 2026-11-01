package observability

import (
	"crypto/x509"
	"errors"
	"sync/atomic"
)

// certificateFaultReported is set once an untrusted-certificate failure has
// been sent to Sentry in this process.
var certificateFaultReported atomic.Bool

// seenCertificateFault reports whether err is a certificate-trust failure that
// was already reported by this process.
//
// A pilot behind antivirus HTTPS scanning or a corporate proxy re-signs every
// connection with a root the machine does not trust, and the chat poll then
// fails on every tick: AIRSPACE-ACARS-H and -J are 312 identical events from
// that one cause. The first report is the diagnosis; the other 311 only bury
// real faults, so later ones are absorbed. Transient deliberately stays false
// for these errors, because the first one must be reported and the pilot
// cannot be told apart from a misconfigured tenant certificate without it.
func seenCertificateFault(err error) bool {
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var hostname x509.HostnameError
	if !errors.As(err, &unknown) && !errors.As(err, &invalid) && !errors.As(err, &hostname) {
		return false
	}
	return !certificateFaultReported.CompareAndSwap(false, true)
}
