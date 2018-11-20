package postgis

import (
	"fmt"

	"github.com/kucjac/imposm3/log"
)

type ColumnType interface {
	Name() string
	PrepareInsertSql(i int,
		spec *TableSpec) string
	GeneralizeSql(colSpec *ColumnSpec, spec *GeneralizedTableSpec) string
}

type simpleColumnType struct {
	name string
}

func (t *simpleColumnType) Name() string {
	return t.name
}

func (t *simpleColumnType) PrepareInsertSql(i int, spec *TableSpec) string {
	return fmt.Sprintf("$%d", i)
}

func (t *simpleColumnType) GeneralizeSql(colSpec *ColumnSpec, spec *GeneralizedTableSpec) string {
	return "\"" + colSpec.Name + "\""
}

type geometryType struct {
	name string
}

func (t *geometryType) Name() string {
	return t.name
}

func (t *geometryType) PrepareInsertSql(i int, spec *TableSpec) string {
	return fmt.Sprintf("$%d::Geometry",
		i,
	)
}

func (t *geometryType) GeneralizeSql(colSpec *ColumnSpec, spec *GeneralizedTableSpec) string {
	return fmt.Sprintf(`ST_SimplifyPreserveTopology("%s", %f) as "%s"`,
		colSpec.Name, spec.Tolerance, colSpec.Name,
	)
}

type validatedGeometryType struct {
	geometryType
}

func (t *validatedGeometryType) GeneralizeSql(colSpec *ColumnSpec, spec *GeneralizedTableSpec) string {
	if spec.Source.GeometryType != "polygon" {
		// TODO return warning earlier
		log.Printf("[warn] validated_geometry column returns polygon geometries for %s", spec.FullName)
	}
	return fmt.Sprintf(`ST_Buffer(ST_SimplifyPreserveTopology("%s", %f), 0) as "%s"`,
		colSpec.Name, spec.Tolerance, colSpec.Name,
	)
}

type h3GeometryType struct {
	geometryType
	indexedResolutions []int8
}

var pgTypes map[string]ColumnType

func init() {
	pgTypes = map[string]ColumnType{
		"string":             &simpleColumnType{"VARCHAR"},
		"bool":               &simpleColumnType{"BOOL"},
		"int8":               &simpleColumnType{"SMALLINT"},
		"int32":              &simpleColumnType{"INT"},
		"int64":              &simpleColumnType{"BIGINT"},
		"float32":            &simpleColumnType{"REAL"},
		"hstore_string":      &simpleColumnType{"HSTORE"},
		"geometry":           &geometryType{"GEOMETRY"},
		"validated_geometry": &validatedGeometryType{geometryType{"GEOMETRY"}},
		"h3geometry":         &h3GeometryType{geometryType: geometryType{"GEOMETRY"}},
	}
}

var defaultH3Resolution int8 = 5
