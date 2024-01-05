package patching

import (
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"strconv"
)

type PatchChange struct {
	FromIndex    int
	ToIndex      int
	LengthChange int
	Text         string
}

var PatchSyntaxError = fmt.Errorf("patch syntax error")
var OutOfPatchBoundsError = fmt.Errorf("patch out of patch bounds")
var OutOfTextBoundsError = fmt.Errorf("patch out of text bounds")

func GetPatchChanges(proposal Proposal, length int) ([]PatchChange, error) {
	pindex := 0
	tindex := 0
	var ret []PatchChange
	for pindex < len(proposal.Patch) {
		match := re.FindStringSubmatch(proposal.Patch[pindex:])
		if match == nil {
			return nil, PatchSyntaxError
		}
		pindex += len(match[0])
		if len(match[1]) < 1 {
			match[1] = match[3]
			match[2] = match[4]
		}
		l, _ := strconv.Atoi(match[2])
		switch match[1] {
		case "+":
			if pindex+l > len(proposal.Patch) {
				return nil, OutOfPatchBoundsError
			}
			ret = append(ret, PatchChange{
				FromIndex:    tindex,
				ToIndex:      tindex - 1,
				LengthChange: l,
				Text:         proposal.Patch[pindex : pindex+l],
			})

			pindex += l
			break
		case "=":
			if tindex+l > length {
				return nil, OutOfTextBoundsError
			}
			tindex += l
		case "-":
			if tindex+l > length {
				return nil, OutOfTextBoundsError
			}
			ret = append(ret, PatchChange{
				FromIndex:    tindex,
				ToIndex:      tindex + l - 1,
				LengthChange: -l,
				Text:         "",
			})
			tindex += l
		}
	}
	return ret, nil
}

func ApplyChanges(text string, changes []PatchChange) string {
	ret := text
	offset := 0

	for _, change := range changes {
		ret = ret[:change.FromIndex+offset] + change.Text + ret[change.ToIndex+1+offset:]
		offset += change.LengthChange
	}
	return ret
}

func PatchString(text string, proposal Proposal) (string, Patch, error) {
	changes, err := GetPatchChanges(proposal, len(text))
	if err != nil {
		return "", Patch{}, err
	}

	ret := ApplyChanges(text, changes)

	return ret, Patch{
		Patch:        proposal.Patch,
		ReversePatch: GenerateProposal(ret, text).Patch,
	}, nil
}

func GenerateProposal(old, new string) Proposal {
	ret := ""
	dmp := diffmatchpatch.New()

	diffs := dmp.DiffMain(old, new, false)
	for _, diff := range diffs {
		switch diff.Type {
		case diffmatchpatch.DiffEqual:
			ret += "=" + strconv.Itoa(len(diff.Text))
			break
		case diffmatchpatch.DiffInsert:
			ret += "+" + strconv.Itoa(len(diff.Text)) + "/" + diff.Text
			break
		case diffmatchpatch.DiffDelete:
			ret += "-" + strconv.Itoa(len(diff.Text))
		}
	}
	return Proposal{Patch: ret}
}
