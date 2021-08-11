// +build go1.11
// +build !go1.13

package gettercheck

import (
	"fmt"
	"strings"
)

func fmtTags(tags []string) string {
	return fmt.Sprintf("-tags=%s", strings.Join(tags, " "))
}
