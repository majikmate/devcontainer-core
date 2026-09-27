package devcontainer

// StripJSONC turns JSON with comments (the format of devcontainer.json) into
// plain JSON: it removes // and /* */ comments and commas before a closing
// bracket. Text inside strings stays unchanged.
func StripJSONC(in []byte) []byte {
	out := make([]byte, 0, len(in))
	inString, escaped := false, false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(in) && in[i+1] == '/':
			for i < len(in) && in[i] != '\n' {
				i++
			}
			if i < len(in) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(in) && in[i+1] == '*':
			i += 2
			for i+1 < len(in) && !(in[i] == '*' && in[i+1] == '/') {
				i++
			}
			i++
		default:
			out = append(out, c)
		}
	}
	return removeTrailingCommas(out)
}

// removeTrailingCommas removes a comma that is followed (after white space)
// by } or ]. The input contains no comments.
func removeTrailingCommas(in []byte) []byte {
	out := make([]byte, 0, len(in))
	inString, escaped := false, false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
		}
		if c == ',' {
			j := i + 1
			for j < len(in) && (in[j] == ' ' || in[j] == '\t' || in[j] == '\n' || in[j] == '\r') {
				j++
			}
			if j < len(in) && (in[j] == '}' || in[j] == ']') {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}
