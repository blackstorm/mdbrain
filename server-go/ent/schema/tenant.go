package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Tenant struct {
	ent.Schema
}

func (Tenant) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entsql.Annotation{Table: "tenants"},
	}
}

func (Tenant) Fields() []ent.Field {
	return []ent.Field{
		stringIDField(),
		field.String("name").NotEmpty(),
		createdAtField(),
	}
}

func (Tenant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
		edge.To("vaults", Vault.Type),
		edge.To("notes", Note.Type),
		edge.To("assets", Asset.Type),
	}
}
