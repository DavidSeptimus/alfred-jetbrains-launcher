package ide

// Resolve picks the installed IDE that should open a project, given the
// project's recorded production code and source version dir, plus an optional
// keyword family that hard-limits the choice to one product family.
//
// Resolution order:
//  1. keyword hard-limit: if keywordFamily set, only that family is considered;
//     prefer the exact recorded version, else the newest in that family.
//  2. exact code + recorded version installed (open in the same version).
//  3. exact code, newest installed version.
//  4. same family, newest installed.
//  5. IDEA-first-class fallback: newest IntelliJ IDEA Ultimate, if installed.
//  6. last resort: newest IDEA Ultimate, else newest installed IDE of any kind.
//
// Returns ok=false only when no IDE at all is installed.
func Resolve(installed []Installed, code, sourceDataDir, keywordFamily string) (Installed, bool) {
	if len(installed) == 0 {
		return Installed{}, false
	}

	if keywordFamily != "" {
		if _, ok := NewestByFamily(installed, keywordFamily); ok {
			// The keyword's IDE is installed: hard-limit to it (exact recorded
			// version within the family if present, else the newest in it).
			if ide, ok := matchVersion(installed, sourceDataDir, func(i Installed) bool {
				return i.Family == keywordFamily
			}); ok {
				return ide, true
			}
			return NewestByFamily(installed, keywordFamily)
		}
		// The keyword's IDE is NOT installed: relax the hard-limit and fall
		// through to the normal chain, so e.g. `goland` still opens Go projects
		// in IntelliJ IDEA Ultimate.
		keywordFamily = ""
	}

	family := FamilyOf(code)

	// 2. exact code + recorded version.
	if ide, ok := matchVersion(installed, sourceDataDir, func(i Installed) bool {
		return i.Code == code
	}); ok {
		return ide, true
	}
	// 3. exact code, newest.
	if code != "" {
		if ide, ok := NewestByCode(installed, code); ok {
			return ide, true
		}
	}
	// 4. same family, newest.
	if family != "" {
		if ide, ok := NewestByFamily(installed, family); ok {
			return ide, true
		}
	}
	// 5. IDEA-first-class fallback.
	if IsIDEAFirstClass(family) {
		if ide, ok := NewestByCode(installed, "IU"); ok {
			return ide, true
		}
	}
	// 6. last resort.
	if ide, ok := NewestByCode(installed, "IU"); ok {
		return ide, true
	}
	return newestMatch(installed, func(Installed) bool { return true })
}

// matchVersion returns the IDE matching pred whose DataDir equals sourceDataDir.
func matchVersion(installed []Installed, sourceDataDir string, pred func(Installed) bool) (Installed, bool) {
	if sourceDataDir == "" {
		return Installed{}, false
	}
	for _, i := range installed {
		if pred(i) && i.DataDir == sourceDataDir {
			return i, true
		}
	}
	return Installed{}, false
}
