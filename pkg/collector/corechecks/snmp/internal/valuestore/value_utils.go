package valuestore

var strippableSpecialChars = map[byte]bool{'\r': true, '\n': true, '\t': true}

func isString(bytesValue []byte) bool {
	for _, bit := range bytesValue {
		if bit < 32 || bit > 126 {
			// The char is not a printable ASCII char but it might be a character that
			// can be stripped like `\n`
			if _, ok := strippableSpecialChars[bit]; !ok {
				return false
			}
		}
	}
	return true
}
