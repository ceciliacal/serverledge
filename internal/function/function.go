package function

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"

	"time"

	"github.com/serverledge-faas/serverledge/internal/cache"
	"github.com/serverledge-faas/serverledge/utils"
	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/net/context"
)

// Function describes a serverless function.
type Function struct {
	Name            string
	Runtime         string   // example: python314
	MemoryMB        int64    // MB
	CPUDemand       float64  // 1.0 -> 1 core
	MaxConcurrency  int16    // intra-container maximum concurrency
	Handler         string   // example: "module.function_name"
	TarFunctionCode string   // input is .tar
	CustomImage     string   // used if custom runtime is chosen
	SupportedArchs  []string // list of supported architectures by the runtime
	Signature       *Signature
	IsDefault       bool    // true when this function is the default/original implementation
	DefaultFunction string  // non-empty when this function is a variant of the named default function
	SpeedUp         float64 // expected speedup over the default implementation; must be > 0 for variants
	Utility         float64 // quality/accuracy utility in [0,1] for variants
	jsonFields      functionJSONFields
}

var (
	ErrInvalidVariantMetadata = errors.New("invalid function variant metadata")
)

type functionJSONFields struct {
	isDefault       bool
	defaultFunction bool
	speedUp         bool
	utility         bool
}

func (f *Function) UnmarshalJSON(data []byte) error {
	type functionAlias Function

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var decoded functionAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*f = Function(decoded)
	f.jsonFields = functionJSONFields{
		isDefault:       jsonFieldPresent(raw, "IsDefault"),
		defaultFunction: jsonFieldPresent(raw, "DefaultFunction"),
		speedUp:         jsonFieldPresent(raw, "SpeedUp"),
		utility:         jsonFieldPresent(raw, "Utility"),
	}
	return nil
}

func jsonFieldPresent(raw map[string]json.RawMessage, name string) bool {
	if _, ok := raw[name]; ok {
		return true
	}
	for key := range raw {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func (f *Function) getEtcdKey() string {
	return getEtcdKey(f.Name)
}

func getEtcdKey(funcName string) string {
	return fmt.Sprintf("/function/%s", funcName)
}

func (f *Function) SupportsArch(arch string) bool {
	return slices.Contains(f.SupportedArchs, arch)
}

func (f *Function) IsVariant() bool {
	return f != nil && f.DefaultFunction != ""
}

func (f *Function) ValidateVariantMetadata() error {
	return f.validateVariantMetadata(false)
}

func (f *Function) ValidateVariantMetadataFromJSON() error {
	return f.validateVariantMetadata(true)
}

func (f *Function) validateVariantMetadata(requireExplicitJSONMetadata bool) error {
	if f == nil {
		return nil
	}
	if f.IsDefault && f.DefaultFunction != "" {
		return fmt.Errorf("%w: default function cannot reference another default function", ErrInvalidVariantMetadata)
	}
	if !f.IsVariant() && f.jsonFields.isDefault && !f.IsDefault {
		return fmt.Errorf("%w: variant default function is required", ErrInvalidVariantMetadata)
	}
	if f.DefaultFunction == "" {
		return nil
	}
	if requireExplicitJSONMetadata {
		if !f.jsonFields.defaultFunction {
			return fmt.Errorf("%w: variant default function is required", ErrInvalidVariantMetadata)
		}
		if !f.jsonFields.speedUp {
			return fmt.Errorf("%w: variant speedup is required", ErrInvalidVariantMetadata)
		}
		if !f.jsonFields.utility {
			return fmt.Errorf("%w: variant utility is required", ErrInvalidVariantMetadata)
		}
	}
	if f.Name == "" {
		return fmt.Errorf("%w: variant name is required", ErrInvalidVariantMetadata)
	}
	if f.Name == f.DefaultFunction {
		return fmt.Errorf("%w: variant name must differ from default function", ErrInvalidVariantMetadata)
	}
	if f.SpeedUp <= 0 {
		return fmt.Errorf("%w: speedup must be greater than zero", ErrInvalidVariantMetadata)
	}
	if f.Utility < 0 || f.Utility > 1 {
		return fmt.Errorf("%w: utility must be in [0,1]", ErrInvalidVariantMetadata)
	}
	return nil
}

func HasVariants(f *Function) bool {
	if f == nil {
		return false
	}
	variants, err := GetVariantsOf(f.Name)
	return err == nil && len(variants) > 0
}

// GetVariantsOf returns all functions explicitly registered as variants of the
// supplied default/original function name.
func GetVariantsOf(baseName string) ([]*Function, error) {
	names, err := GetAll()
	if err != nil {
		return nil, err
	}

	functions := make([]*Function, 0, len(names))
	for _, name := range names {
		f, ok := GetFunction(name)
		if !ok || f == nil {
			continue
		}
		functions = append(functions, f)
	}
	return filterVariants(baseName, functions), nil
}

func filterVariants(baseName string, functions []*Function) []*Function {
	variants := make([]*Function, 0)
	for _, f := range functions {
		if f != nil && f.IsVariant() && f.DefaultFunction == baseName {
			variants = append(variants, f)
		}
	}
	sort.Slice(variants, func(i, j int) bool {
		return variants[i].Name < variants[j].Name
	})
	return variants
}

func GetVariantNamesOf(baseName string) ([]string, error) {
	variants, err := GetVariantsOf(baseName)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(variants))
	for _, f := range variants {
		if f == nil {
			continue
		}
		names = append(names, f.Name)
	}
	return names, nil
}

// GetFunction retrieves a Function given its name. If it doesn't exist, returns false
func GetFunction(name string) (*Function, bool) {

	val, found := getFromCache(name)
	if !found {
		// cache miss
		f, response := getFromEtcd(name)
		if !response {
			return nil, false
		}
		//insert a new element to the cache
		cache.GetCacheInstance().Set(name, f, cache.DefaultExp)
		return f, true
	}

	return val, true

}

func (f *Function) String() string {
	return f.Name
}

func getFromCache(name string) (*Function, bool) {
	localCache := cache.GetCacheInstance()
	f, found := localCache.Get(name)
	if !found {
		return nil, false
	}
	//cache hit
	//return a safe copy of the function previously obtained
	function := *f.(*Function)
	return &function, true

}

func getFromEtcd(name string) (*Function, bool) {
	cli, err := utils.GetEtcdClient()
	if err != nil {
		return nil, false
	}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	getResponse, err := cli.Get(ctx, getEtcdKey(name))
	if err != nil {
		utils.TriggerEtcdReconnection()
		log.Printf("etcd get failed: %v", err)
		return nil, false
	} else if len(getResponse.Kvs) < 1 {
		return nil, false
	}

	var f Function
	err = json.Unmarshal(getResponse.Kvs[0].Value, &f)
	if err != nil {
		return nil, false
	}

	return &f, true
}

// SaveToEtcd registers the function to Etcd
func (f *Function) SaveToEtcd() error {
	if err := f.ValidateVariantMetadata(); err != nil {
		return err
	}

	cli, err := utils.GetEtcdClient()
	if err != nil {
		return err
	}
	ctx := context.TODO()

	payload, err := json.Marshal(*f)
	if err != nil {
		return fmt.Errorf("Could not marshal function: %v", err)
	}
	_, err = cli.Put(ctx, f.getEtcdKey(), string(payload))
	if err != nil {
		utils.TriggerEtcdReconnection()
		return fmt.Errorf("Failed Put: %v", err)
	}

	// Add the function to the local cache
	cache.GetCacheInstance().Set(f.Name, f, cache.DefaultExp)

	return nil
}

// Delete removes a function from Etcd and the local cache.
func (f *Function) Delete() error {
	cli, err := utils.GetEtcdClient()
	if err != nil {
		return err
	}
	ctx := context.TODO()

	dresp, err := cli.Delete(ctx, f.getEtcdKey())
	if err != nil {
		return fmt.Errorf("Failed Delete: %v", err)
	} else if dresp.Deleted != 1 {
		fmt.Printf("no function with key '%s' exists", f.getEtcdKey())
	}

	// Remove the function from the local cache
	cache.GetCacheInstance().Delete(f.Name)

	return nil
}

func (f *Function) Equals(f2 *Function) bool {
	return (f == nil && f2 == nil) || (f.Name == f2.Name &&
		f.CustomImage == f2.CustomImage &&
		f.CPUDemand == f2.CPUDemand &&
		f.Runtime == f2.Runtime &&
		f.Handler == f2.Handler &&
		f.MemoryMB == f2.MemoryMB &&
		f.TarFunctionCode == f2.TarFunctionCode)
}

// Exists checks if the function is already saved to Etcd
func (f *Function) Exists() bool {
	savedFunction, ok := GetFunction(f.Name)
	return ok && f.Equals(savedFunction)
}

// GetAll returns all function names
func GetAll() ([]string, error) {
	return GetAllWithPrefix("/function")
}

// GetAllWithPrefix is used to get all /function or /workflow currently registered in etcd
func GetAllWithPrefix(prefix string) ([]string, error) {
	cli, err := utils.GetEtcdClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	resp, err := cli.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	functions := make([]string, len(resp.Kvs))
	for i, s := range resp.Kvs {
		functions[i] = string(s.Key)[len(prefix+"/"):]
	}

	return functions, ctx.Err()
}
