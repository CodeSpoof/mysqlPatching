package patching

import (
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"strconv"
)

type patchChange struct {
	fromIndex    int
	toIndex      int
	lengthChange int
	text         string
}

var PatchSyntaxError = fmt.Errorf("patch syntax error")
var OutOfPatchBoundsError = fmt.Errorf("patch out of patch bounds")
var OutOfTextBoundsError = fmt.Errorf("patch out of text bounds")

func getPatchChanges(proposal Proposal, length int) ([]patchChange, error) {
	pindex := 0
	tindex := 0
	var ret []patchChange
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
			ret = append(ret, patchChange{
				fromIndex:    tindex,
				toIndex:      tindex - 1,
				lengthChange: l,
				text:         proposal.Patch[pindex : pindex+l],
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
			ret = append(ret, patchChange{
				fromIndex:    tindex,
				toIndex:      tindex + l - 1,
				lengthChange: -l,
				text:         "",
			})
			tindex += l
		}
	}
	return ret, nil
}

func applyChanges(text string, changes []patchChange) string {
	ret := text
	offset := 0

	for _, change := range changes {
		ret = ret[:change.fromIndex+offset] + change.text + ret[change.toIndex+1+offset:]
		offset += change.lengthChange
	}
	return ret
}

func patchString(text string, proposal Proposal) (string, Patch, error) {
	changes, err := getPatchChanges(proposal, len(text))
	if err != nil {
		return "", Patch{}, err
	}

	ret := applyChanges(text, changes)

	return ret, Patch{
		Patch:        proposal.Patch,
		ReversePatch: generateProposal(ret, text).Patch,
	}, nil
}

func generateProposal(old, new string) Proposal {
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
