package tftp

import "testing"

func TestPackUnpack(t *testing.T) {
	v := []string{"test-filename/with-subdir"}
	testOptsList := []options{
		nil,
		{
			"tsize":   "1234",
			"blksize": "22",
		},
	}
	for _, filename := range v {
		for _, mode := range []string{"octet", "netascii"} {
			for _, opts := range testOptsList {
				packUnpack(t, filename, mode, opts)
			}
		}
	}
}

func packUnpack(t *testing.T, filename, mode string, opts options) {
	b := make([]byte, datagramLength)
	for _, op := range []uint16{opRRQ, opWRQ} {
		n := packRQ(b, op, filename, mode, opts)
		f, m, o, err := unpackRQ(b[:n])
		if err != nil {
			t.Errorf("%s pack/unpack: %v", filename, err)
		}
		if f != filename {
			t.Errorf("filename mismatch (%s): '%x' vs '%x'",
				filename, f, filename)
		}
		if m != mode {
			t.Errorf("mode mismatch (%s): '%x' vs '%x'",
				mode, m, mode)
		}
		for name, value := range opts {
			v, ok := o[name]
			if !ok {
				t.Errorf("missing %s option", name)
			}
			if v != value {
				t.Errorf("option %s mismatch: '%x' vs '%x'", name, v, value)
			}
		}
	}
}
