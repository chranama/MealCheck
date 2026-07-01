package localmodel

func localLlamaMealName(code string) (string, bool) {
	switch code {
	case "b":
		return "breakfast", true
	case "m":
		return "morning snack", true
	case "l":
		return "lunch", true
	case "a":
		return "afternoon snack", true
	case "d":
		return "dinner", true
	case "s":
		return "snack", true
	case "e":
		return "evening snack", true
	default:
		return "", false
	}
}
func localLlamaMealRank(code string) int {
	switch code {
	case "b":
		return 0
	case "m":
		return 1
	case "l":
		return 2
	case "a":
		return 3
	case "d":
		return 4
	case "s":
		return 5
	case "e":
		return 6
	default:
		return 99
	}
}
