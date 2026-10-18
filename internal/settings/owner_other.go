//go:build !unix

package settings

// adoptOwner вне unix ничего не делает: владельцев по uid там нет.
func adoptOwner([]string) error { return nil }
