package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type NoteAssetRef struct {
	ent.Schema
}

func (NoteAssetRef) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entsql.Annotation{Table: "note_asset_refs"},
	}
}

func (NoteAssetRef) Fields() []ent.Field {
	return []ent.Field{
		stringIDField(),
		field.String("vault_id").NotEmpty(),
		field.String("note_client_id").NotEmpty(),
		field.String("asset_client_id").NotEmpty(),
		createdAtField(),
	}
}

func (NoteAssetRef) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("vault", Vault.Type).
			Ref("note_asset_refs").
			Field("vault_id").
			Required().
			Unique(),
	}
}

func (NoteAssetRef) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("vault_id", "note_client_id", "asset_client_id").Unique(),
		index.Fields("vault_id", "note_client_id"),
		index.Fields("vault_id", "asset_client_id"),
	}
}
