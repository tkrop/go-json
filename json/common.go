package json

// hexmap maps every ASCII byte to its 4-bit hex value. Entries outside
// '0'-'9', 'A'-'F', 'a'-'f' are 0xFF (invalid sentinel). Valid values are
// `0x00`-`0x0F`; the OR of any four values can therefore be compared to be
// larger than `0x0F` in a single branch to detect any invalid digit.
var hexmap = func() [256]byte {
	t := [256]byte{}
	for i := range t {
		t[i] = 0xFF
	}
	for c := byte('0'); c <= '9'; c++ {
		t[c] = c - '0'
	}
	for c := byte('A'); c <= 'F'; c++ {
		t[c] = c - 'A' + 10
	}
	for c := byte('a'); c <= 'f'; c++ {
		t[c] = c - 'a' + 10
	}
	return t
}()

// unihex decodes exactly 4 hexadecimal digits from the start of the buffer
// into a rune using a lookup table for branch-free per-digit decoding. The
// function returns (rune, true) on success or (0, false) on bad input. The
// caller must ensure that the buffer has at least 4 bytes. (e.g. by calling
// `s.reader.extend()` before invoking `unihex`).
func unihex(b []byte) (rune, bool) {
	v0, v1, v2, v3 := hexmap[b[0]], hexmap[b[1]], hexmap[b[2]], hexmap[b[3]]

	if v0|v1|v2|v3 > 0x0F {
		return 0, false
	}

	return rune(v0)<<12 | rune(v1)<<8 | rune(v2)<<4 | rune(v3), true
}

// uspace checks for UTF-8 encoded whitespace at the given offset in the
// window. It returns the number of bytes in the whitespace sequence, a boolean
// indicating if it is a line terminator, and a boolean indicating if more
// bytes are needed to determine the whitespace character.
//
//nolint:gocognit // prioritize performance.
func uspace(window []byte, offset int) (int, bool, bool) {
	if window[offset] == 0xC2 {
		if offset+1 >= len(window) {
			return 0, false, true
		} else if window[offset+1] == 0xA0 {
			return 2, false, false
		}
	}

	//nolint:nestif // prioritize performance.
	if window[offset] == 0xEF {
		if offset+1 >= len(window) {
			return 0, false, true
		} else if window[offset+1] == 0xBB {
			if offset+2 >= len(window) {
				return 0, false, true
			} else if window[offset+2] == 0xBF {
				return 3, false, false
			}
		}
	}

	//nolint:nestif // prioritize performance.
	if window[offset] == 0xE2 {
		if offset+1 >= len(window) {
			return 0, false, true
		} else if window[offset+1] == 0x80 {
			if offset+2 >= len(window) {
				return 0, false, true
			} else if window[offset+2] == 0xA8 ||
				window[offset+2] == 0xA9 {
				return 3, true, false
			}
		}
	}

	return 0, false, false
}
