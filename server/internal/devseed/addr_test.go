package devseed

import "testing"

func TestIsLoopbackAddr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8090", true},
		{"localhost:8090", true},
		{"[::1]:8090", true},
		{"0.0.0.0:8090", false},
		{":8090", false},
		{"192.168.1.10:8090", false},
		{"", false},
		{"not-an-addr", false},
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			t.Parallel()
			if got := IsLoopbackAddr(tc.addr); got != tc.want {
				t.Fatalf("IsLoopbackAddr(%q) = %v, want %v", tc.addr, got, tc.want)
			}
		})
	}
}

func TestRequireLoopback(t *testing.T) {
	t.Parallel()
	if err := RequireLoopback("127.0.0.1:8090"); err != nil {
		t.Fatal(err)
	}
	if err := RequireLoopback("0.0.0.0:8090"); err == nil {
		t.Fatal("0.0.0.0 を許可してはいけない")
	}
}
