package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type NoteLink struct {
	ent.Schema
}

func (NoteLink) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entsql.Annotation{Table: "note_links"},
	}
}

func (NoteLink) Fields() []ent.Field {
	return []ent.Field{
		stringIDField(),
		field.String("vault_id").NotEmpty(),
		field.String("source_client_id").NotEmpty(),
		field.String("target_client_id").NotEmpty(),
		field.String("target_path").Optional().Nillable(),
		field.String("link_type").NotEmpty(),
		field.String("display_text").Optional().Nillable(),
		field.String("original").Optional().Nillable(),
		createdAtField(),
		updatedAtField(),
	}
}

func (NoteLink) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("vault", Vault.Type).
			Ref("note_links").
			Field("vault_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Required().
			Unique(),
	}
}

func (NoteLink) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("vault_id", "source_client_id"),
		index.Fields("vault_id", "target_client_id"),
		index.Fields("vault_id", "source_client_id", "target_client_id"),
	}
}
