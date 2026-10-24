package passwordhash

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	a, err := Hash("correct password")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Hash("correct password")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("password salts must differ")
	}
	if !Verify("correct password", a) || Verify("incorrect password", a) {
		t.Fatal("incorrect verification")
	}
	for _, bad := range []string{"", "$argon2id$v=19$m=0,t=1,p=0$AA$AA", "$argon2id$v=19$m=4294967295,t=1,p=4$AA$AA", "$argon2id$v=18$m=65536,t=1,p=4$AA$AA"} {
		if Verify("anything", bad) {
			t.Fatal("accepted malformed hash")
		}
	}
}
