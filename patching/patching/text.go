package patching

import (
	"database/sql"
	"fmt"
	"github.com/CodeSpoof/goCommon/database"
	"github.com/google/uuid"
	"os"
	"regexp"
	"slices"
)

var re = regexp.MustCompile(`^([-=])(\d+)|(\+)(\d+)/`)

type Text struct {
	Uuid        string
	Content     string
	LastPatchId int64
	Owner       int64
}

type Patch struct {
	Id           int64
	Ranking      int64
	Patch        string
	ReversePatch string
	Owner        int64
	Message      string
}

type Proposal struct {
	Id          int64
	LastPatchId int64
	Patch       string
	Owner       int64
	Message     string
}

var PatchIncompatibleError error = fmt.Errorf("patch no longer compatible")
var TimelineMismatchError error = fmt.Errorf("timeline mismatch in proposal")

func Open(uuid string, db database.DbLike) (*Text, error) {
	t := Text{Uuid: uuid}
	row := db.QueryRow("SELECT content, IFNULL(patching_patches.id, 0), patching_texts.owner FROM patching_texts LEFT OUTER JOIN patching_patches ON patching_texts.uuid=patching_patches.text_uuid WHERE uuid=?1 AND patching_patches.ranking=(SELECT MAX(ranking) FROM patching_patches WHERE text_uuid=?1 GROUP BY text_uuid)", t.Uuid)
	if err := row.Scan(&t.Content, &t.LastPatchId, &t.Owner); err != nil {
		return nil, err
	}
	return &t, nil
}

func NewText(owner int64, db database.DbLike) (*Text, error) {
	t := Text{
		Uuid:        uuid.NewString(),
		Content:     "",
		LastPatchId: 0,
		Owner:       owner,
	}
	_, err := db.Exec("INSERT INTO patching_texts (uuid, content, owner) VALUES (?, '', ?)", t.Uuid, t.Owner)
	if err != nil {
		return nil, err
	}
	proposal, err := t.Propose("", "", 0, db)
	if err != nil {
		return nil, err
	}
	err = t.Accept(*proposal, db)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (text *Text) Accept(proposal Proposal, db database.DbLike) error {
	if text.LastPatchId != proposal.LastPatchId {
		return TimelineMismatchError
	}
	var err error
	var p Patch
	text.Content, p, err = PatchString(text.Content, proposal)
	p.Owner = proposal.Owner
	p.Message = proposal.Message
	if err != nil {
		return err
	}
	var r int64
	row := db.QueryRow("SELECT IFNULL((SELECT MAX(ranking) FROM patching_patches WHERE text_uuid=? GROUP BY text_uuid), 0)+1", text.Uuid)
	if err = row.Scan(&r); err != nil {
		return err
	}
	exec, err := db.Exec("INSERT INTO patching_patches (text_uuid, ranking, patch, reverse_patch, owner, message) VALUES (?, ?, ?, ?, ?, ?)", text.Uuid, r, p.Patch, p.ReversePatch, p.Owner, p.Message)
	if err != nil {
		return err
	}
	id, err := exec.LastInsertId()
	if err != nil {
		return err
	}
	text.LastPatchId = id
	_, err = db.Exec("UPDATE patching_texts SET content=? WHERE uuid=?", text.Content, text.Uuid)
	if err != nil {
		return err
	}
	if proposal.Id != 0 {
		_, err := db.Exec("DELETE FROM patching_proposals WHERE id=?", proposal.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (text *Text) Propose(newText string, message string, owner int64, db database.DbLike) (*Proposal, error) {
	p := GenerateProposal(text.Content, newText)
	result, err := db.Exec("INSERT INTO patching_proposals (text_uuid, last_patch, patch, owner, message) VALUES (?, ?, ?, ?, ?)", text.Uuid, text.LastPatchId, p.Patch, owner, message)
	if err != nil {
		return nil, err
	}
	p.Id, err = result.LastInsertId()
	if err != nil {
		return nil, err
	}
	p.LastPatchId = text.LastPatchId
	p.Owner = owner
	p.Message = message
	return &p, nil
}

func (text *Text) GetPatches(db database.DbLike) ([]Patch, error) {
	rows, err := db.Query("SELECT id, ranking, patch, reverse_patch, owner, message FROM patching_patches WHERE text_uuid=? AND ranking <= (SELECT ranking FROM patching_patches WHERE id=?) ORDER BY ranking", text.Uuid, text.LastPatchId)
	if err != nil {
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			os.Exit(1)
		}
	}(rows)
	var ret []Patch
	for rows.Next() {
		p := Patch{}
		err = rows.Scan(&p.Id, &p.Ranking, &p.Patch, &p.ReversePatch, &p.Owner, &p.Message)
		if err != nil {
			return ret, err
		}
		ret = append(ret, p)
	}
	return ret, nil
}

func (text *Text) GetProposals(db database.DbLike) ([]Proposal, error) {
	rows, err := db.Query("SELECT pr.id, pr.last_patch, pr.patch, pr.owner, pr.message FROM patching_proposals pr JOIN patching_patches pa ON pr.text_uuid=pa.text_uuid AND pr.last_patch=pa.id WHERE pr.text_uuid=? ORDER BY pa.ranking", text.Uuid)
	if err != nil {
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			os.Exit(1)
		}
	}(rows)
	var ret []Proposal
	for rows.Next() {
		p := Proposal{}
		err = rows.Scan(&p.Id, &p.LastPatchId, &p.Patch, &p.Owner, &p.Message)
		if err != nil {
			return ret, err
		}
		ret = append(ret, p)
	}
	return ret, nil
}

func (text *Text) GetOldVersion(lastPatch Patch, db database.DbLike) (*Text, error) {
	patches, err := text.GetPatches(db)
	if err != nil {
		return &Text{}, err
	}
	slices.Reverse(patches)
	ret := ""
	for _, patch := range patches {
		if patch.Id == lastPatch.Id {
			break
		}
		ret, _, err = PatchString(text.Content, Proposal{Patch: patch.ReversePatch})
		if err != nil {
			return &Text{}, err
		}
	}
	return &Text{
		Uuid:        text.Uuid,
		Content:     ret,
		LastPatchId: lastPatch.Id,
		Owner:       text.Owner,
	}, nil
}

func (text *Text) UpdateProposal(proposal Proposal, db database.DbLike) (*Proposal, error) {
	patches, err := text.GetPatches(db)
	if err != nil {
		return nil, err
	}
	if len(patches) < 1 {
		return &proposal, nil
	}
	i := len(patches) - 1
	for ; i >= 0; i-- {
		if patches[i].Id == proposal.LastPatchId {
			break
		}
	}
	old, err := text.GetOldVersion(patches[i], db)
	if err != nil {
		return nil, err
	}
	myChanges, err := GetPatchChanges(proposal, len(old.Content))
	if err != nil {
		return nil, err
	}
	for i++; i < len(patches); i++ {
		realChanges, err := GetPatchChanges(Proposal{Patch: patches[i].Patch}, len(old.Content))
		if err != nil {
			return nil, err
		}
		offset := 0
		procUntil := 0
		for _, realChange := range realChanges {
			for i2, myChange := range myChanges[procUntil:] {
				if myChange.FromIndex > realChange.ToIndex {
					break
				}
				if realChange.FromIndex > myChange.ToIndex {
					myChanges[i2+procUntil].FromIndex += offset
					myChanges[i2+procUntil].ToIndex += offset
					procUntil++
					continue
				}
				return nil, PatchIncompatibleError
			}
			offset += realChange.LengthChange
		}
		for i3 := range myChanges[procUntil:] {
			myChanges[i3+procUntil].FromIndex += offset
			myChanges[i3+procUntil].ToIndex += offset
		}
		old.Content, _, err = PatchString(old.Content, Proposal{Patch: patches[i].Patch})
		if err != nil {
			return nil, err
		}
		old.LastPatchId = patches[i].Id
	}
	p := GenerateProposal(text.Content, ApplyChanges(text.Content, myChanges))
	p.Id = proposal.Id
	p.LastPatchId = text.LastPatchId
	p.Owner = proposal.Owner
	p.Message = proposal.Message
	return &p, nil
}
