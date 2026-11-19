package safety

import "testing"

func TestScan(t *testing.T) {
	bad := []string{
		"the key is AKIAIOSFODNN7EXAMPLE ok",
		"password: hunter2hunter2",
		"temp password is Sunshine42!",
		"-----BEGIN RSA PRIVATE KEY-----",
		"connect to https://admin:secretpw@host/x",
		"token=abcd1234efgh",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N",
		"use k9Fj3Kd8sLq2Xz7Vb1Nm4Qw6",
	}
	for _, s := range bad {
		if len(Scan(s)) == 0 {
			t.Errorf("expected finding in %q", s)
		}
	}
	good := []string{
		"Printer at Acme is fixed, waiting on toner",
		"Reset the password for jsmith and emailed the link (T20260928.0042)",
		"disk at 95% on ACME-SRV01, watch it",
	}
	for _, s := range good {
		if f := Scan(s); len(f) != 0 {
			t.Errorf("false positive in %q: %v", s, f)
		}
	}
	for _, f := range Scan("password: hunter2hunter2") {
		if f.Sample == "hunter2hunter2" {
			t.Error("finding must be redacted")
		}
	}
}
