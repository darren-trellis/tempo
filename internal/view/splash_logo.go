package view

import (
	"math/rand"
	"strings"
	"unicode/utf8"
)

// SplashLogo is one of the ASCII wordmarks the splash screen draws.
type SplashLogo struct {
	Name string
	Art  string
}

const splashLogoBlocks = `
░▒▓████████▓▒░▒▓████████▓▒░▒▓██████████████▓▒░░▒▓███████▓▒░ ░▒▓██████▓▒░  
   ░▒▓█▓▒░   ░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░ 
   ░▒▓█▓▒░   ░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░ 
   ░▒▓█▓▒░   ░▒▓██████▓▒░ ░▒▓█▓▒░░▒▓█▓▒░░▒▓█▓▒░▒▓███████▓▒░░▒▓█▓▒░░▒▓█▓▒░ 
   ░▒▓█▓▒░   ░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░ 
   ░▒▓█▓▒░   ░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░      ░▒▓█▓▒░░▒▓█▓▒░ 
   ░▒▓█▓▒░   ░▒▓████████▓▒░▒▓█▓▒░░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░       ░▒▓██████▓▒░  
`

const splashLogoTerrace = `
░██████████░██████████ ░███     ░███ ░█████████    ░██████   
    ░██    ░██         ░████   ░████ ░██     ░██  ░██   ░██  
    ░██    ░██         ░██░██ ░██░██ ░██     ░██ ░██     ░██ 
    ░██    ░█████████  ░██ ░████ ░██ ░█████████  ░██     ░██ 
    ░██    ░██         ░██  ░██  ░██ ░██         ░██     ░██ 
    ░██    ░██         ░██       ░██ ░██          ░██   ░██  
    ░██    ░██████████ ░██       ░██ ░██           ░██████   
`

const splashLogoDoh = `
TTTTTTTTTTTTTTTTTTTTTTTEEEEEEEEEEEEEEEEEEEEEEMMMMMMMM               MMMMMMMMPPPPPPPPPPPPPPPPP        OOOOOOOOO     
T:::::::::::::::::::::TE::::::::::::::::::::EM:::::::M             M:::::::MP::::::::::::::::P     OO:::::::::OO   
T:::::::::::::::::::::TE::::::::::::::::::::EM::::::::M           M::::::::MP::::::PPPPPP:::::P  OO:::::::::::::OO 
T:::::TT:::::::TT:::::TEE::::::EEEEEEEEE::::EM:::::::::M         M:::::::::MPP:::::P     P:::::PO:::::::OOO:::::::O
TTTTTT  T:::::T  TTTTTT  E:::::E       EEEEEEM::::::::::M       M::::::::::M  P::::P     P:::::PO::::::O   O::::::O
        T:::::T          E:::::E             M:::::::::::M     M:::::::::::M  P::::P     P:::::PO:::::O     O:::::O
        T:::::T          E::::::EEEEEEEEEE   M:::::::M::::M   M::::M:::::::M  P::::PPPPPP:::::P O:::::O     O:::::O
        T:::::T          E:::::::::::::::E   M::::::M M::::M M::::M M::::::M  P:::::::::::::PP  O:::::O     O:::::O
        T:::::T          E:::::::::::::::E   M::::::M  M::::M::::M  M::::::M  P::::PPPPPPPPP    O:::::O     O:::::O
        T:::::T          E::::::EEEEEEEEEE   M::::::M   M:::::::M   M::::::M  P::::P            O:::::O     O:::::O
        T:::::T          E:::::E             M::::::M    M:::::M    M::::::M  P::::P            O:::::O     O:::::O
        T:::::T          E:::::E       EEEEEEM::::::M     MMMMM     M::::::M  P::::P            O::::::O   O::::::O
      TT:::::::TT      EE::::::EEEEEEEE:::::EM::::::M               M::::::MPP::::::PP          O:::::::OOO:::::::O
      T:::::::::T      E::::::::::::::::::::EM::::::M               M::::::MP::::::::P           OO:::::::::::::OO 
      T:::::::::T      E::::::::::::::::::::EM::::::M               M::::::MP::::::::P             OO:::::::::OO   
      TTTTTTTTTTT      EEEEEEEEEEEEEEEEEEEEEEMMMMMMMM               MMMMMMMMPPPPPPPPPP               OOOOOOOOO
`

// SplashLogos are the wordmarks the splash picks from, narrowest first.
var SplashLogos = []SplashLogo{
	{Name: "terrace", Art: splashLogoTerrace},
	{Name: "blocks", Art: splashLogoBlocks},
	{Name: "doh", Art: splashLogoDoh},
}

// splashLogoChrome is the rows the splash spends on everything but the logo
// itself: a row of padding above and below it, the status block, the sponsor
// line and its spacer.
const splashLogoChrome = 7

// Lines returns the art as rows, without the blank leading and trailing lines
// the raw literals carry.
func (l SplashLogo) Lines() []string {
	return strings.Split(strings.Trim(l.Art, "\n"), "\n")
}

// Height is the number of rows the art occupies.
func (l SplashLogo) Height() int { return len(l.Lines()) }

// Width is the number of columns the widest row occupies.
func (l SplashLogo) Width() int {
	widest := 0
	for _, line := range l.Lines() {
		if n := utf8.RuneCountInString(line); n > widest {
			widest = n
		}
	}
	return widest
}

// splashLogosFitting returns the wordmarks that render whole in a terminal of
// this size. A zero column or row count means the size is unknown, so that
// dimension does not rule anything out.
func splashLogosFitting(cols, rows int) []SplashLogo {
	fits := make([]SplashLogo, 0, len(SplashLogos))
	for _, logo := range SplashLogos {
		if cols > 0 && logo.Width() > cols {
			continue
		}
		if rows > 0 && logo.Height()+splashLogoChrome > rows {
			continue
		}
		fits = append(fits, logo)
	}
	return fits
}

// RandomSplashLogo picks a wordmark at random from the ones that fit a
// terminal of this size. When none fit it falls back to the narrowest, since
// a clipped wordmark still beats a blank splash.
func RandomSplashLogo(cols, rows int) SplashLogo {
	fits := splashLogosFitting(cols, rows)
	if len(fits) == 0 {
		return SplashLogos[0]
	}
	return fits[rand.Intn(len(fits))]
}
