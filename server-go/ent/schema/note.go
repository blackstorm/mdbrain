package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Note struct {
	ent.Schema
}

func (Note) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entsql.Annotation{Table: "notes"},
	}
}

func (Note) Fields() []ent.Field {
	return []ent.Field{
		stringIDField(),
		field.String("tenant_id").NotEmpty(),
		field.String("vault_id").NotEmpty(),
		field.String("path").NotEmpty(),
		field.String("client_id").NotEmpty(),
		field.String("content").Optional().Nillable(),
		field.String("metadata").Optional().Nillable(),
		field.String("hash").Optional().Nillable(),
		field.String("mtime").Optional().Nillable(),
		field.Int64("deleted_at").Optional().Nillable(),
		createdAtField(),
		updatedAtField(),
	}
}

func (Note) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tenant", Tenant.Type).
			Ref("notes").
			Field("tenant_id").
			Required().
			Unique(),
		edge.From("vault", Vault.Type).
			Ref("notes").
			Field("vault_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Required().
			Unique(),
	}
}

func (Note) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("vault_id", "client_id").Unique(),
		index.Fields("vault_id"),
		index.Fields("vault_id", "path"),
		index.Fields("mtime"),
	}
}
