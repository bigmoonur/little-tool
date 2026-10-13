//go:build production

package main

import "testing"

func TestProductionNeverBypassesLicensing(t *testing.T) {
	if licenseBypass {
		t.Fatal("production build contains a licensing bypass")
	}
}
