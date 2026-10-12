package run

// OrderSpecs returns the process-list order: core APIs, core frontends, apps
// grouped with each backend before its frontend, then infrastructure.
// Catalog order is preserved within sections and for each app's first appearance.
// The original slice and specs are not modified.
func OrderSpecs(specs []*Spec) []*Spec {
	var core, web, infra []*Spec
	var appOrder []string
	apps := map[string][]*Spec{}
	for _, spec := range specs {
		switch {
		case spec.Kind == KindInfra:
			infra = append(infra, spec)
		case spec.Group != "":
			if _, known := apps[spec.Group]; !known {
				appOrder = append(appOrder, spec.Group)
			}
			apps[spec.Group] = append(apps[spec.Group], spec)
		case spec.Kind == KindAPI:
			core = append(core, spec)
		default:
			web = append(web, spec)
		}
	}
	ordered := make([]*Spec, 0, len(specs))
	ordered = append(ordered, core...)
	ordered = append(ordered, web...)
	for _, group := range appOrder {
		for _, spec := range apps[group] {
			if spec.Kind == KindAPI {
				ordered = append(ordered, spec)
			}
		}
		for _, spec := range apps[group] {
			if spec.Kind != KindAPI {
				ordered = append(ordered, spec)
			}
		}
	}
	return append(ordered, infra...)
}
