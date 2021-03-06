package distro

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/osbuild/osbuild-composer/internal/osbuild"

	"github.com/osbuild/osbuild-composer/internal/blueprint"
	"github.com/osbuild/osbuild-composer/internal/rpmmd"
)

// A Distro represents composer's notion of what a given distribution is.
type Distro interface {
	// Returns the name of the distro.
	Name() string

	// Returns the module platform id of the distro. This is used by DNF
	// for modularity support.
	ModulePlatformID() string

	// Returns a sorted list of the names of the architectures this distro
	// supports.
	ListArches() []string

	// Returns an object representing the given architecture as support
	// by this distro.
	GetArch(arch string) (Arch, error)
}

// An Arch represents a given distribution's support for a given architecture.
type Arch interface {
	// Returns the name of the architecture.
	Name() string

	// Returns a sorted list of the names of the image types this architecture
	// supports.
	ListImageTypes() []string

	// Returns an object representing a given image format for this architecture,
	// on this distro.
	GetImageType(imageType string) (ImageType, error)

	// Returns the parent distro
	Distro() Distro
}

// An ImageType represents a given distribution's support for a given Image Type
// for a given architecture.
type ImageType interface {
	// Returns the name of the image type.
	Name() string

	// Returns the parent architecture
	Arch() Arch

	// Returns the canonical filename for the image type.
	Filename() string

	// Retrns the MIME-type for the image type.
	MIMEType() string

	// Returns the proper image size for a given output format. If the input size
	// is 0 the default value for the format will be returned.
	Size(size uint64) uint64

	// Returns the default packages to include and exclude when making the image
	// type.
	BasePackages() ([]string, []string)

	// Returns the build packages for the output type.
	BuildPackages() []string

	// Returns an osbuild manifest, containing the sources and pipeline necessary
	// to build an image, given output format with all packages and customizations
	// specified in the given blueprint.
	Manifest(b *blueprint.Customizations, options ImageOptions, repos []rpmmd.RepoConfig, packageSpecs, buildPackageSpecs []rpmmd.PackageSpec) (*osbuild.Manifest, error)
}

// The ImageOptions specify options for a specific image build
type ImageOptions struct {
	OSTree OSTreeImageOptions
	Size   uint64
}

// The OSTreeImageOptions specify ostree-specific image options
type OSTreeImageOptions struct {
	Ref    string
	Parent string
}

type Registry struct {
	distros map[string]Distro
}

func NewRegistry(distros ...Distro) (*Registry, error) {
	reg := &Registry{
		distros: make(map[string]Distro),
	}
	for _, distro := range distros {
		name := distro.Name()
		if _, exists := reg.distros[name]; exists {
			return nil, fmt.Errorf("NewRegistry: passed two distros with the same name: %s", distro.Name())
		}
		reg.distros[name] = distro
	}
	return reg, nil
}

func (r *Registry) GetDistro(name string) Distro {
	distro, ok := r.distros[name]
	if !ok {
		return nil
	}

	return distro
}

// List returns the names of all distros in a Registry, sorted alphabetically.
func (r *Registry) List() []string {
	list := []string{}
	for _, distro := range r.distros {
		list = append(list, distro.Name())
	}
	sort.Strings(list)
	return list
}

func (r *Registry) FromHost() (Distro, error) {
	name, err := GetHostDistroName()
	if err != nil {
		return nil, err
	}

	d := r.GetDistro(name)
	if d == nil {
		return nil, errors.New("unknown distro: " + name)
	}

	return d, nil
}

func GetHostDistroName() (string, error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "", err
	}
	defer f.Close()
	osrelease, err := readOSRelease(f)
	if err != nil {
		return "", err
	}

	// NOTE: We only consider major releases
	version := strings.Split(osrelease["VERSION_ID"], ".")
	name := osrelease["ID"] + "-" + version[0]
	return name, nil
}

func readOSRelease(r io.Reader) (map[string]string, error) {
	osrelease := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, errors.New("readOSRelease: invalid input")
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if value[0] == '"' {
			if len(value) < 2 || value[len(value)-1] != '"' {
				return nil, errors.New("readOSRelease: invalid input")
			}
			value = value[1 : len(value)-1]
		}

		osrelease[key] = value
	}

	return osrelease, nil
}
