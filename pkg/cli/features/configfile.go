package features

import (
	"context"
	"os"
	"reflect"
	"slices"

	"github.com/fatih/structtag"
	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
	"github.com/gravitational/trace"
	"github.com/invopop/jsonschema"
	"github.com/jinzhu/copier"
	"github.com/spf13/cobra"
)

// Gets a config file path from the command line and reads the config file into a struct of type T.
// The flag is required.
// The struct is then validated using the "jsonschema" and "validate" tags.
type ConfigFileCommand[T interface{}] struct {
	ConfigFilePath string
}

func (cfc *ConfigFileCommand[T]) ConfigureFlags(cmd *cobra.Command) {
	const flagName = "config-file"
	cmd.Flags().StringVarP(&cfc.ConfigFilePath, flagName, "c", "", "Path to the configuration file to use for CLI requests.")
	cmd.MarkFlagFilename(flagName)
	cmd.MarkFlagRequired(flagName)
}

// TODO move the  translation logic to a separate package, or maybe a separate module. This could be useful
// in other projects.

// Processes a field in a struct, updating the "validate" tag if the "jsonschema" tag is set to "required".
func processField(field reflect.StructField) (reflect.StructField, error) {
	const (
		jsonSchemaTagName = "jsonschema"
		validationTagName = "validate"
	)

	tags, err := structtag.Parse(string(field.Tag))
	if err != nil {
		return field, trace.Wrap(err, "failed to parse struct tags for field %q", field.Name)
	}

	tagKeys := tags.Keys()
	if !slices.Contains(tagKeys, jsonSchemaTagName) {
		return field, nil
	}

	jsonSchemaTag, err := tags.Get(jsonSchemaTagName)
	if err != nil {
		// This shouldn't happen due to the keys check above, but error checking is needed in case
		// the upstream library changes
		return field, trace.Wrap(err, "failed to get %s tag for field %q", jsonSchemaTagName, field.Name)
	}

	if jsonSchemaTag == nil {
		return field, nil
	}

	shouldMarkRequired := jsonSchemaTag.Name == "required" || jsonSchemaTag.HasOption("required")
	if !shouldMarkRequired {
		return field, nil
	}

	var validatorTag *structtag.Tag
	if slices.Contains(tagKeys, validationTagName) {
		validatorTag, err = tags.Get(validationTagName)
		if err != nil {
			return field, trace.Wrap(err, "failed to get %s tag for field %q", validationTagName, field.Name)
		}
	}
	if validatorTag == nil {
		validatorTag = &structtag.Tag{
			Key: validationTagName,
		}
	}

	if validatorTag.Name == "" {
		validatorTag.Name = "required"
	} else {
		validatorTag.Options = append(validatorTag.Options, "required")
	}

	err = tags.Set(validatorTag)
	if err != nil {
		// This should only occur if the key isn't set, but error checking is needed in case the upstream library changes
		return field, trace.Wrap(err, "failed to set validate tag for field %q", field.Name)
	}

	field.Tag = reflect.StructTag(tags.String())
	return field, nil
}

// Given a reflect.Type referring to a struct, translates the "jsonschema" tags to "validate" tags.
func translateJsonSchemaTagsToValidateTags(reflectType reflect.Type) (reflect.Type, error) {
	if reflectType.Kind() != reflect.Struct {
		return nil, trace.BadParameter("expected a struct type, got %q", reflectType.Kind())
	}

	fieldCount := reflectType.NumField()
	if fieldCount == 0 {
		// No need to create a new type without a concrete backing type
		return reflectType, nil
	}

	fields := make([]reflect.StructField, 0, fieldCount)
	for fieldNum := range fieldCount {
		field, err := processField(reflectType.Field(fieldNum))
		if err != nil {
			return nil, trace.Wrap(err, "failed to process field %q", field.Name)
		}

		if field.Type.Kind() == reflect.Struct {
			translatedType, err := translateJsonSchemaTagsToValidateTags(field.Type)
			if err != nil {
				return nil, trace.Wrap(err, "failed to translate field %q", field.Name)
			}
			field.Type = translatedType
		}

		fields = append(fields, field)
	}

	translatedType := reflect.StructOf(fields)
	return translatedType, nil
}

// Given a struct of type T, create an instance of a new type with the "jsonschema" tags translated to "validate" tags.
// All field names will be the same. Fields that are structs will be recursively translated, and may also be a new type.
func newTranslatedConfigStruct[T interface{}]() (interface{}, error) {
	var defaultVal T
	reflectType := reflect.TypeOf(defaultVal)

	translatedType, err := translateJsonSchemaTagsToValidateTags(reflectType)
	if err != nil {
		return defaultVal, trace.Wrap(err, "failed to translate tags")
	}

	return reflect.New(translatedType).Elem().Interface(), nil
}

// Given a set of configuration values, validate the configuration using the "validate" tags.
// Some JSON schema tags are also supported.
func (cfc *ConfigFileCommand[T]) validateConfig(ctx context.Context, config T) error {
	translated, err := newTranslatedConfigStruct[T]()
	if err != nil {
		return trace.Wrap(err, "failed to translate jsonschema tags to validate tags")
	}

	// Copying the values from the original config to the translated config provides the
	// ability to validate the original config with JSON schema tag support, without modifying
	// the underlying type.
	err = copier.Copy(&translated, &config)
	if err != nil {
		return trace.Wrap(err, "failed to copy values from config to translated config")
	}

	configValidator := validator.New(validator.WithRequiredStructEnabled())
	return trace.Wrap(configValidator.StructCtx(ctx, translated))
}

// Read a configuration file provided via CLI flag, validate it, and return the loaded configuration.
func (cfc *ConfigFileCommand[T]) ReadConfigFile(ctx context.Context) (T, error) {
	var defaultVal T

	configFileContents, err := os.ReadFile(cfc.ConfigFilePath)
	if err != nil {
		return defaultVal, trace.Wrap(err, "failed to read config file %q", cfc.ConfigFilePath)
	}

	config := *new(T)
	err = yaml.UnmarshalContext(ctx, configFileContents, &config, yaml.Strict(), yaml.AllowDuplicateMapKey())
	if err != nil {
		return defaultVal, trace.Wrap(err, "failed to unmarshal config file %q", cfc.ConfigFilePath)
	}

	err = cfc.validateConfig(ctx, config)
	if err != nil {
		return defaultVal, trace.Wrap(err, "failed to validate config file %q", cfc.ConfigFilePath)
	}

	return config, nil
}

func (cfc *ConfigFileCommand[T]) GenerateConfigSchema() ([]byte, error) {
	configInstance := new(T)
	schemaReflector := &jsonschema.Reflector{
		RequiredFromJSONSchemaTags: true,
	}
	schema, err := schemaReflector.Reflect(configInstance).MarshalJSON()
	return schema, trace.Wrap(err, "failed to marshal schema for %T", configInstance)
}
