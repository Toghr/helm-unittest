package unittest

import (
	"fmt"
	"reflect"

	"github.com/lrills/helm-unittest/internal/common"
	"github.com/lrills/helm-unittest/pkg/unittest/results"
	"github.com/lrills/helm-unittest/pkg/unittest/validators"

	"github.com/mitchellh/mapstructure"
)

// Assertion defines target and metrics to validate rendered result
type Assertion struct {
	Template         string
	DocumentIndex    int
	Not              bool
	AssertType       string
	validator        validators.Validatable
	antonym          bool
	defaultTemplates []string
	Kind 			 string
}

// returns list of templates that match Kind from Assertion
// for example all 'Deployments'
func (a *Assertion) ChooseTemplatesByKind(
	templatesResult map[string][]common.K8sManifest,
	snapshotComparer validators.SnapshotComparer,
) []string {
	fmt.Fprintf(os.Stdout, "Searching for templates to validate by kind: %s\n", a.Kind)

	templatesToValidate := make([]string, 0)

	kindValidator := new(validators.IsKindValidator)
	kindValidator.Of = a.Kind

	for template, rendered := range templatesResult {
		ctx := &validators.ValidateContext{
			Docs:             rendered,
			Index:            a.DocumentIndex,
			Negative:         a.Not != a.antonym,
			SnapshotComparer: snapshotComparer,
		}

		var validatePassed bool
		validatePassed, _ = kindValidator.Validate(ctx)
		fmt.Fprintf(os.Stdout, "Template: %s", template)
		if validatePassed {
			fmt.Fprintf(os.Stdout, " MATCH\n")
			templatesToValidate = append(templatesToValidate, template)
		} else {
			fmt.Fprintf(os.Stdout,  " SKIP\n")
		}

	}

	fmt.Fprintf(os.Stdout, "Templates to verify: %s\n", templatesToValidate)
	return templatesToValidate
}

// Assert validate the rendered manifests with validator
func (a *Assertion) Assert(
	templatesResult map[string][]common.K8sManifest,
	snapshotComparer validators.SnapshotComparer,
	result *results.AssertionResult,
) *results.AssertionResult {
	result.AssertType = a.AssertType
	result.Not = a.Not

	// Ensure assertion is succeeding or failing based on templates to test.
	assertionPassed := false
	failInfo := make([]string, 0)

	if a.Kind != "" {
		fmt.Fprintf(os.Stdout, "Checking kinds: %s\n", a.Kind)
		//a.ChooseTemplatesByKind(templatesResult, snapshotComparer)
		a.defaultTemplates = a.ChooseTemplatesByKind(templatesResult, snapshotComparer)
	}

	for idx, template := range a.defaultTemplates {
		rendered, ok := templatesResult[template]
		var validatePassed bool
		var singleFailInfo []string
		if !ok {
			noFile := []string{"Error:", a.noFileErrMessage(template)}
			failInfo = append(failInfo, noFile...)
			assertionPassed = false
			break
		}

		validatePassed, singleFailInfo = a.validator.Validate(&validators.ValidateContext{
			Docs:             rendered,
			Index:            a.DocumentIndex,
			Negative:         a.Not != a.antonym,
			SnapshotComparer: snapshotComparer,
		})

		if !validatePassed {
			failInfoTemplate := []string{fmt.Sprintf("Template:\t%s", template)}
			singleFailInfo = append(failInfoTemplate, singleFailInfo...)
		}

		if idx == 0 {
			assertionPassed = true
		}

		assertionPassed = assertionPassed && validatePassed
		failInfo = append(failInfo, singleFailInfo...)
	}

	result.Passed = assertionPassed
	result.FailInfo = failInfo

	return result
}

func (a *Assertion) noFileErrMessage(template string) string {
	if template != "" {
		return fmt.Sprintf(
			"\ttemplate \"%s\" not exists or not selected in test suite",
			template,
		)
	}

	return "\tassertion.template must be given if testsuite.templates is empty"
}

// UnmarshalYAML implement yaml.Unmalshaler, construct validator according to the assert type
func (a *Assertion) UnmarshalYAML(unmarshal func(interface{}) error) error {
	assertDef := make(map[string]interface{})
	if err := unmarshal(&assertDef); err != nil {
		return err
	}

	if documentIndex, ok := assertDef["documentIndex"].(int); ok {
		a.DocumentIndex = documentIndex
	} else {
		a.DocumentIndex = -1
	}

	if not, ok := assertDef["not"].(bool); ok {
		a.Not = not
	}

	if template, ok := assertDef["template"].(string); ok {
		a.Template = template
	}

	if err := a.constructValidator(assertDef); err != nil {
		return err
	}

	if a.validator == nil {
		for key := range assertDef {
			if key != "template" && key != "documentIndex" && key != "not" {
				return fmt.Errorf("Assertion type `%s` is invalid", key)
			}
		}
		return fmt.Errorf("No assertion type defined")
	}

	return nil
}

func (a *Assertion) constructValidator(assertDef map[string]interface{}) error {
	for assertName, correspondDef := range assertTypeMapping {
		if params, ok := assertDef[assertName]; ok {
			if a.validator != nil {
				return fmt.Errorf(
					"Assertion type `%s` and `%s` is declared duplicately",
					a.AssertType,
					assertName,
				)
			}

			validator := reflect.New(correspondDef.validatorType).Interface()
			if err := mapstructure.Decode(params, validator); err != nil {
				return err
			}

			a.AssertType = assertName
			a.validator = validator.(validators.Validatable)
			a.antonym = correspondDef.antonym
			a.defaultTemplates = []string{a.Template}
		}
	}
	return nil
}

type assertTypeDef struct {
	validatorType reflect.Type
	antonym       bool
}

var assertTypeMapping = map[string]assertTypeDef{
	"matchSnapshot":     {reflect.TypeOf(validators.MatchSnapshotValidator{}), false},
	"matchSnapshotRaw":  {reflect.TypeOf(validators.MatchSnapshotRawValidator{}), false},
	"equal":             {reflect.TypeOf(validators.EqualValidator{}), false},
	"notEqual":          {reflect.TypeOf(validators.EqualValidator{}), true},
	"equalRaw":          {reflect.TypeOf(validators.EqualRawValidator{}), false},
	"notEqualRaw":       {reflect.TypeOf(validators.EqualRawValidator{}), true},
	"matchRegex":        {reflect.TypeOf(validators.MatchRegexValidator{}), false},
	"notMatchRegex":     {reflect.TypeOf(validators.MatchRegexValidator{}), true},
	"matchRegexRaw":     {reflect.TypeOf(validators.MatchRegexRawValidator{}), false},
	"notMatchRegexRaw":  {reflect.TypeOf(validators.MatchRegexRawValidator{}), true},
	"contains":          {reflect.TypeOf(validators.ContainsValidator{}), false},
	"notContains":       {reflect.TypeOf(validators.ContainsValidator{}), true},
	"isNull":            {reflect.TypeOf(validators.IsNullValidator{}), false},
	"isNotNull":         {reflect.TypeOf(validators.IsNullValidator{}), true},
	"isEmpty":           {reflect.TypeOf(validators.IsEmptyValidator{}), false},
	"isNotEmpty":        {reflect.TypeOf(validators.IsEmptyValidator{}), true},
	"isKind":            {reflect.TypeOf(validators.IsKindValidator{}), false},
	"isAPIVersion":      {reflect.TypeOf(validators.IsAPIVersionValidator{}), false},
	"hasDocuments":      {reflect.TypeOf(validators.HasDocumentsValidator{}), false},
	"isSubset":          {reflect.TypeOf(validators.IsSubsetValidator{}), false},
	"isNotSubset":       {reflect.TypeOf(validators.IsSubsetValidator{}), true},
	"failedTemplate":    {reflect.TypeOf(validators.FailedTemplateValidator{}), false},
	"notFailedTemplate": {reflect.TypeOf(validators.FailedTemplateValidator{}), true},
}
