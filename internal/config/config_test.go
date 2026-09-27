package config

import "testing"

func valid() Input {
	return Input{
		NodeIP:        "192.168.1.117",
		Domain:        "naslos.local",
		AdminUser:     "admin",
		AdminPassword: "Correct1",
	}
}

func TestValidateAcceptsGoodInput(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Input){
		"bad ip":            func(i *Input) { i.NodeIP = "not-an-ip" },
		"ipv6":              func(i *Input) { i.NodeIP = "fe80::1" },
		"empty domain":      func(i *Input) { i.Domain = "" },
		"domain with slash": func(i *Input) { i.Domain = "naslos.local/evil" },
		"bad domain label":  func(i *Input) { i.Domain = "-naslos.local" },
		"bad uid":           func(i *Input) { i.AdminUser = "1admin" },
		"uid with space":    func(i *Input) { i.AdminUser = "ad min" },
		"short pw":          func(i *Input) { i.AdminPassword = "Ab1" },
		"pw no digit":       func(i *Input) { i.AdminPassword = "Password" },
		"pw no upper":       func(i *Input) { i.AdminPassword = "password1" },
		"pw no lower":       func(i *Input) { i.AdminPassword = "PASSWORD1" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := valid()
			mutate(&in)
			if err := in.Validate(); err == nil {
				t.Fatalf("want validation error for %s", name)
			}
		})
	}
}

func TestDerivations(t *testing.T) {
	in := valid()
	if got := in.ShortName(); got != "naslos" {
		t.Fatalf("ShortName = %q", got)
	}
	if got, err := in.Subnet24(); err != nil {
		t.Fatalf("Subnet24: %v", err)
	} else if got != "192.168.1.0/24" {
		t.Fatalf("Subnet24 = %q", got)
	}
	if got := in.AutheliaURL(); got != "https://naslos.local/authelia" {
		t.Fatalf("AutheliaURL = %q", got)
	}
	in.Name = "homenas"
	if got := in.ShortName(); got != "homenas" {
		t.Fatalf("ShortName with override = %q", got)
	}
}

func TestShortNameTruncatesLongLabel(t *testing.T) {
	in := valid()
	in.Domain = "averyverylonglabel.local"
	if got := in.ShortName(); len(got) != 15 {
		t.Fatalf("ShortName = %q (len %d)", got, len(got))
	}
}
