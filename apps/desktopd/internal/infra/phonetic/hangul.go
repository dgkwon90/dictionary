package phonetic

const (
	hangulBase = 0xAC00
	hangulEnd  = 0xD7A3
)

var choseongPhones = [...]string{
	"G", "G", "N", "D", "D", "R", "M", "B", "B", "S", "S", "", "JH", "JH", "CH", "K", "T", "P", "HH",
}

var jungseongPhones = [...][]string{
	{"AA"},
	{"AE"},
	{"Y", "AA"},
	{"Y", "AE"},
	{"AH"},
	{"EH"},
	{"Y", "AH"},
	{"Y", "EH"},
	{"OW"},
	{"W", "AA"},
	{"W", "AE"},
	{"W", "EH"},
	{"Y", "OW"},
	{"UW"},
	{"W", "AH"},
	{"W", "EH"},
	{"W", "IY"},
	{"Y", "UW"},
	{},
	{"IY"},
	{"IY"},
}

var jongseongPhones = [...]string{
	"", "K", "K", "K", "N", "N", "N", "T", "L", "K", "M", "L", "L", "L", "P", "L", "M", "P", "P",
	"S", "S", "NG", "T", "CH", "K", "T", "P", "",
}

func hangulToARPAbet(input string) []string {
	phones := make([]string, 0, len(input))
	for _, r := range input {
		if r < hangulBase || r > hangulEnd {
			continue
		}

		syllable := r - hangulBase
		cho := int(syllable / 588)
		jung := int((syllable % 588) / 28)
		jong := int(syllable % 28)

		if phone := choseongPhones[cho]; phone != "" {
			phones = append(phones, phone)
		}
		for _, phone := range jungseongPhones[jung] {
			if phone != "" {
				phones = append(phones, phone)
			}
		}
		if phone := jongseongPhones[jong]; phone != "" {
			phones = append(phones, phone)
		}
	}
	return phones
}
