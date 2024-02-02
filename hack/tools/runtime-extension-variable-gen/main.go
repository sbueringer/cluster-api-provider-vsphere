/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// main is the main package for runtime-extension-variable-gen.
package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"
	"golang.org/x/exp/slices"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-tools/pkg/crd"
	"sigs.k8s.io/controller-tools/pkg/loader"
	"sigs.k8s.io/controller-tools/pkg/markers"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	paths      = flag.String("paths", "", "Paths with the variable types.")
	outputFile = flag.String("output-file", "zz_generated.variables.json", "Output file name.")
	name       = flag.String("name", "Variables", "Name of the go type that holds the variables.")
)

// runtime-extension-variable-gen generates a JSON file with ClusterClass variable schema definitions
// based on Go types. The Go types can be annotated with kubebuilder markers just like regular CRD API Go types.
func main() {
	flag.Parse()

	if *name == "" {
		klog.Exit("--name must be specified")
	}

	if *paths == "" {
		klog.Exit("--paths must be specified")
	}

	if *outputFile == "" {
		klog.Exit("--output-file must be specified")
	}

	outputFileExt := path.Ext(*outputFile)
	if outputFileExt != ".json" {
		klog.Exit("--output-file must have 'json' extension")
	}

	if err := run(*name, *paths, *outputFile); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// The following code is inspired by the CRD generator in controller-gen.
// The CRD generator there parses CRD API Go types and generates the corresponding
// CRD YAMLs that are then used to deploy the CRDs to Kubernetes.
// We took the relevant parts from the CRD generator to parse the variable types
// but instead of generated a CRD YAML we generate a JSON file which contains a marshalled
// array of clusterv1.ClusterClassVariables.
func run(name, paths, outputFile string) error {
	// Create CRD Generator
	crdGen := crd.Generator{}

	// Load the configured packages.
	rootPackages, err := loader.LoadRoots(paths)
	if err != nil {
		fmt.Println(err)
	}

	// Setup the marker collector.
	collector := &markers.Collector{
		Registry: &markers.Registry{},
	}
	if err = crdGen.RegisterMarkers(collector.Registry); err != nil {
		return err
	}

	// Setup the CRD parser.
	parser := &crd.Parser{
		Collector: collector,
		Checker: &loader.TypeChecker{
			NodeFilters: []loader.NodeFilter{crdGen.CheckFilter()},
		},
		IgnoreUnexportedFields:     true,
		AllowDangerousTypes:        false,
		GenerateEmbeddedObjectMeta: false,
	}
	crd.AddKnownTypes(parser)

	// Add packages to the parser.
	for _, root := range rootPackages {
		parser.NeedPackage(root)
	}

	// Go through the found types and add the ones where the struct name is equal to 'name'
	// to variableGroupKinds.
	variableGroupKinds := []schema.GroupKind{}
	for typeIdent := range parser.Types {
		// If we need another way to identify "variable structs": look at: crd.FindKubeKinds(parser, metav1Pkg)
		if typeIdent.Name == name {
			variableGroupKinds = append(variableGroupKinds, schema.GroupKind{
				Group: parser.GroupVersions[typeIdent.Package].Group,
				Kind:  typeIdent.Name,
			})
		}
	}

	// Iterate through variableGroupKinds:
	// * Find the apiExtensionsSchema for each variable
	// * Convert the apiExtensionsSchema to a ClusterClassVariable
	// Inspired by: parser.NeedCRDFor(groupKind, nil)
	var variables []clusterv1.ClusterClassVariable
	for _, variableGroupKind := range variableGroupKinds {
		// Find the apiExtensionsSchema for each variable.
		var packages []*loader.Package
		for pkg, gv := range parser.GroupVersions {
			if gv.Group != variableGroupKind.Group {
				continue
			}
			// Get package for the current GroupKind
			packages = append(packages, pkg)
		}
		var apiExtensionsSchema *apiextensionsv1.JSONSchemaProps
		for _, pkg := range packages {
			typeIdent := crd.TypeIdent{Package: pkg, Name: variableGroupKind.Kind}
			typeInfo := parser.Types[typeIdent]

			// Didn't find type in pkg.
			if typeInfo == nil {
				continue
			}

			parser.NeedFlattenedSchemaFor(typeIdent)
			fullSchema := parser.FlattenedSchemata[typeIdent]
			apiExtensionsSchema = fullSchema.DeepCopy() // don't mutate the cache (we might be truncating description, etc)
		}

		if apiExtensionsSchema == nil {
			return errors.Errorf("Couldn't find schema for %s", variableGroupKind)
		}

		// Convert the apiExtensionsSchema to a ClusterClassVariable
		for variableName, variableSchema := range apiExtensionsSchema.Properties {
			vs := variableSchema
			openAPIV3Schema, errs := convertToJSONSchemaProps(&vs, field.NewPath("schema"))
			if len(errs) > 0 {
				return errs.ToAggregate()
			}
			variable := clusterv1.ClusterClassVariable{
				Name: variableName,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: *openAPIV3Schema,
				},
			}
			for _, requiredVariable := range apiExtensionsSchema.Required {
				if variableName == requiredVariable {
					variable.Required = true
				}
			}
			variables = append(variables, variable)
		}
	}

	// Sort the variables by name to get a stable output.
	slices.SortFunc(variables, func(a, b clusterv1.ClusterClassVariable) int {
		return cmp.Compare(a.Name, b.Name)
	})

	// Marshal the variables.
	res, err := json.MarshalIndent(variables, "", "  ")
	if err != nil {
		return err
	}

	// Write the variables to 'outputFile'.
	if err := os.WriteFile(outputFile, res, 0600); err != nil {
		return errors.Wrapf(err, "failed to write generated file")
	}

	return nil
}

// JSONSchemaProps converts an apiextensions.JSONSchemaProp to a clusterv1.JSONSchemaProps.
// Note: This is required because a ClusterClassVariable uses clusterv1.JSONSchemaProps
// instead of apiextensions.JSONSchemaProp.
func convertToJSONSchemaProps(schema *apiextensionsv1.JSONSchemaProps, fldPath *field.Path) (*clusterv1.JSONSchemaProps, field.ErrorList) {
	var allErrs field.ErrorList

	props := &clusterv1.JSONSchemaProps{
		Description:            schema.Description,
		Type:                   schema.Type,
		Required:               schema.Required,
		MaxItems:               schema.MaxItems,
		MinItems:               schema.MinItems,
		UniqueItems:            schema.UniqueItems,
		Format:                 schema.Format,
		MaxLength:              schema.MaxLength,
		MinLength:              schema.MinLength,
		Pattern:                schema.Pattern,
		ExclusiveMaximum:       schema.ExclusiveMaximum,
		ExclusiveMinimum:       schema.ExclusiveMinimum,
		Default:                schema.Default,
		Enum:                   schema.Enum,
		Example:                schema.Example,
		XPreserveUnknownFields: ptr.Deref(schema.XPreserveUnknownFields, false),
	}

	if schema.Maximum != nil {
		f := int64(*schema.Maximum)
		props.Maximum = &f
	}

	if schema.Minimum != nil {
		f := int64(*schema.Minimum)
		props.Minimum = &f
	}

	if schema.AdditionalProperties != nil {
		jsonSchemaProps, err := convertToJSONSchemaProps(schema.AdditionalProperties.Schema, fldPath.Child("additionalProperties"))
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("additionalProperties"), "",
				fmt.Sprintf("failed to convert schema: %v", err)))
		} else {
			props.AdditionalProperties = jsonSchemaProps
		}
	}

	if len(schema.Properties) > 0 {
		props.Properties = map[string]clusterv1.JSONSchemaProps{}
		for propertyName, propertySchema := range schema.Properties {
			p := propertySchema
			jsonSchemaProps, err := convertToJSONSchemaProps(&p, fldPath.Child("properties").Key(propertyName))
			if err != nil {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("properties").Key(propertyName), "",
					fmt.Sprintf("failed to convert schema: %v", err)))
			} else {
				props.Properties[propertyName] = *jsonSchemaProps
			}
		}
	}

	if schema.Items != nil {
		jsonSchemaProps, err := convertToJSONSchemaProps(schema.Items.Schema, fldPath.Child("items"))
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("items"), "",
				fmt.Sprintf("failed to convert schema: %v", err)))
		} else {
			props.Items = jsonSchemaProps
		}
	}

	return props, allErrs
}
