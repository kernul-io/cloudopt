package terraform

import "strings"

// ParseModulePath splits a Terraform address into module prefix and resource suffix.
func ParseModulePath(address string) (modulePath, resourceSuffix string) {
	if !strings.HasPrefix(address, "module.") {
		return "", address
	}
	parts := strings.Split(address, ".")
	if len(parts) < 3 {
		return address, ""
	}
	modulePath = parts[0] + "." + parts[1]
	resourceSuffix = strings.Join(parts[2:], ".")
	return modulePath, resourceSuffix
}

// IndexKeyFromAddress extracts count/for_each index from the final resource name segment.
func IndexKeyFromAddress(address string) string {
	lastDot := strings.LastIndex(address, ".")
	seg := address
	if lastDot >= 0 {
		seg = address[lastDot+1:]
	}
	bracket := strings.Index(seg, "[")
	if bracket < 0 {
		return ""
	}
	end := strings.Index(seg[bracket:], "]")
	if end < 0 {
		return ""
	}
	return seg[bracket+1 : bracket+end]
}
