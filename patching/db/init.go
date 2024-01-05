package db

import (
	"github.com/CodeSpoof/goCommon/database"
)

func Init(_db database.DbLike) error {
	_, err := _db.Exec("create table if not exists patching_texts (uuid varchar(36) not null primary key, content text not null, owner int not null);")
	if err != nil {
		return err
	}
	_, err = _db.Exec("create table if not exists patching_proposals (id int auto_increment primary key, text_uuid text not null, last_patch int not null, patch text not null, message text not null, owner int not null);")
	if err != nil {
		return err
	}
	_, err = _db.Exec("create table if not exists patching_patches (id int auto_increment primary key, text_uuid text not null, ranking int not null, patch text not null, reverse_patch text not null, message text not null, owner int not null);")
	return err
}
