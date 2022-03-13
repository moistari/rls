package rls_test

import (
	"fmt"

	"github.com/moistari/rls"
)

func ExampleParse() {
	const title = "The_Velvet_Underground-The_Complete_Matrix_Tapes-Reissue_Limited_Edition_Boxset-8LP-2019-NOiR"
	r := rls.ParseString(title)
	fmt.Printf("%q:\n", r)
	fmt.Printf("  type: %s\n", r.Type)
	fmt.Printf("  artist: %s\n", r.Artist)
	fmt.Printf("  title: %s\n", r.Title)
	fmt.Printf("  year: %d\n", r.Year)
	fmt.Printf("  disc: %s\n", r.Disc)
	fmt.Printf("  source: %s\n", r.Source)
	fmt.Printf("  edition: %q\n", r.Edition)
	fmt.Printf("  other: %q\n", r.Other)
	fmt.Printf("  group: %s\n", r.Group)
	// Output:
	// "The_Velvet_Underground-The_Complete_Matrix_Tapes-Reissue_Limited_Edition_Boxset-8LP-2019-NOiR":
	//   type: music
	//   artist: The Velvet Underground
	//   title: The Complete Matrix Tapes
	//   year: 2019
	//   disc: 8x
	//   source: LP
	//   edition: ["Limited.Edition"]
	//   other: ["REISSUE" "BOXSET"]
	//   group: NOiR
}
