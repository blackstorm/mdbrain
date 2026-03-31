package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Asset struct {
	ent.Schema
}

func (Asset) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entsql.Annotation{Table: "assets"},
	}
}

func (Asset) Fields() []ent.Field {
	return []ent.Field{
		stringIDField(),
		field.String("tenant_id").NotEmpty(),
		field.String("vault_id").NotEmpty(),
		field.String("client_id").NotEmpty(),
		field.String("path").NotEmpty(),
		field.String("object_key").NotEmpty(),
		field.Int64("size_bytes"),
		field.String("content_type").NotEmpty(),
		field.String("md5").NotEmpty(),
		field.Int64("deleted_at").Optional().Nillable(),
		createdAtField(),
		updatedAtField(),
	}
}

func (Asset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tenant", Tenant.Type).
			Ref("assets").
			Field("tenant_id").
			Required().
			Unique(),
		edge.From("vault", Vault.Type).
			Ref("assets").
			Field("vault_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Required().
			Unique(),
	}
}

func (Asset) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("vault_id", "client_id").Unique(),
		index.Fields("vault_id"),
		index.Fields("vault_id", "path"),
		index.Fields("vault_id", "deleted_at"),
		index.Fields("vault_id", "md5"),
	}
}
