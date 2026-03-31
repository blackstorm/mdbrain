package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Vault struct {
	ent.Schema
}

func (Vault) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entsql.Annotation{Table: "vaults"},
	}
}

func (Vault) Fields() []ent.Field {
	return []ent.Field{
		stringIDField(),
		field.String("tenant_id").NotEmpty(),
		field.String("name").NotEmpty(),
		field.String("domain").Optional().Nillable(),
		field.String("sync_key").NotEmpty(),
		field.String("client_type").Default("obsidian"),
		field.String("root_note_id").Optional().Nillable(),
		field.String("logo_object_key").Optional().Nillable(),
		field.String("custom_head_html").Optional().Nillable(),
		field.Time("last_publish_at").Optional().Nillable(),
		field.String("last_publish_status").Default("never"),
		field.String("last_publish_error_code").Optional().Nillable(),
		field.String("last_publish_error_message").Optional().Nillable(),
		createdAtField(),
	}
}

func (Vault) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tenant", Tenant.Type).
			Ref("vaults").
			Field("tenant_id").
			Required().
			Unique(),
		edge.To("notes", Note.Type),
		edge.To("assets", Asset.Type),
		edge.To("note_links", NoteLink.Type),
		edge.To("note_asset_refs", NoteAssetRef.Type),
	}
}

func (Vault) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("domain").Unique(),
		index.Fields("sync_key").Unique(),
	}
}
