package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
)

func stringIDField() ent.Field {
	return field.String("id").
		NotEmpty().
		Immutable()
}

func createdAtField() ent.Field {
	return field.Time("created_at").
		Default(time.Now).
		Immutable().
		Annotations(entsql.Default("CURRENT_TIMESTAMP"))
}

func updatedAtField() ent.Field {
	return field.Time("updated_at").
		Default(time.Now).
		UpdateDefault(time.Now).
		Annotations(entsql.Default("CURRENT_TIMESTAMP"))
}
