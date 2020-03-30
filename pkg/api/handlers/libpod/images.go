package libpod

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/containers/buildah"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/libpod/define"
	"github.com/containers/libpod/libpod/image"
	image2 "github.com/containers/libpod/libpod/image"
	"github.com/containers/libpod/pkg/api/handlers"
	"github.com/containers/libpod/pkg/api/handlers/utils"
	"github.com/containers/libpod/pkg/domain/entities"
	"github.com/containers/libpod/pkg/util"
	utils2 "github.com/containers/libpod/utils"
	"github.com/gorilla/schema"
	"github.com/pkg/errors"
)

// Commit
// author string
// "container"
// repo string
// tag string
// message
// pause bool
// changes []string

// create

func ImageExists(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	name := utils.GetName(r)

	_, err := runtime.ImageRuntime().NewFromLocal(name)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusNotFound, errors.Wrapf(err, "Failed to find image %s", name))
		return
	}
	utils.WriteResponse(w, http.StatusNoContent, "")
}

func ImageTree(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	name := utils.GetName(r)

	img, err := runtime.ImageRuntime().NewFromLocal(name)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusNotFound, errors.Wrapf(err, "Failed to find image %s", name))
		return
	}

	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		WhatRequires bool `schema:"whatrequires"`
	}{
		WhatRequires: false,
	}
	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "failed to parse parameters for %s", r.URL.String()))
		return
	}

	tree, err := img.GenerateTree(query.WhatRequires)
	if err != nil {
		utils.Error(w, "Server error", http.StatusInternalServerError, errors.Wrapf(err, "failed to generate image tree for %s", name))
		return
	}

	utils.WriteResponse(w, http.StatusOK, tree)
}

func GetImage(w http.ResponseWriter, r *http.Request) {
	name := utils.GetName(r)
	newImage, err := utils.GetImage(r, name)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusNotFound, errors.Wrapf(err, "Failed to find image %s", name))
		return
	}
	inspect, err := newImage.Inspect(r.Context())
	if err != nil {
		utils.Error(w, "Server error", http.StatusInternalServerError, errors.Wrapf(err, "failed in inspect image %s", inspect.ID))
		return
	}
	utils.WriteResponse(w, http.StatusOK, inspect)
}

func GetImages(w http.ResponseWriter, r *http.Request) {
	images, err := utils.GetImages(w, r)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "Failed get images"))
		return
	}
	var summaries = make([]*entities.ImageSummary, len(images))
	for j, img := range images {
		is, err := handlers.ImageToImageSummary(img)
		if err != nil {
			utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "Failed transform image summaries"))
			return
		}
		// libpod has additional fields that we need to populate.
		is.Created = img.Created().Unix()
		is.ReadOnly = img.IsReadOnly()
		summaries[j] = is
	}
	utils.WriteResponse(w, http.StatusOK, summaries)
}

func PruneImages(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		All     bool                `schema:"all"`
		Filters map[string][]string `schema:"filters"`
	}{
		// override any golang type defaults
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}

	var libpodFilters = []string{}
	if _, found := r.URL.Query()["filters"]; found {
		dangling := query.Filters["all"]
		if len(dangling) > 0 {
			query.All, err = strconv.ParseBool(query.Filters["all"][0])
			if err != nil {
				utils.InternalServerError(w, err)
				return
			}
		}
		// dangling is special and not implemented in the libpod side of things
		delete(query.Filters, "dangling")
		for k, v := range query.Filters {
			libpodFilters = append(libpodFilters, fmt.Sprintf("%s=%s", k, v[0]))
		}
	}

	cids, err := runtime.ImageRuntime().PruneImages(r.Context(), query.All, libpodFilters)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, err)
		return
	}
	utils.WriteResponse(w, http.StatusOK, cids)
}

func ExportImage(w http.ResponseWriter, r *http.Request) {
	var (
		output string
	)
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Compress bool   `schema:"compress"`
		Format   string `schema:"format"`
	}{
		Format: define.OCIArchive,
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}
	switch query.Format {
	case define.OCIArchive, define.V2s2Archive:
		tmpfile, err := ioutil.TempFile("", "api.tar")
		if err != nil {
			utils.Error(w, "unable to create tmpfile", http.StatusInternalServerError, errors.Wrap(err, "unable to create tempfile"))
			return
		}
		output = tmpfile.Name()
		if err := tmpfile.Close(); err != nil {
			utils.Error(w, "unable to close tmpfile", http.StatusInternalServerError, errors.Wrap(err, "unable to close tempfile"))
			return
		}
	case define.OCIManifestDir, define.V2s2ManifestDir:
		tmpdir, err := ioutil.TempDir("", "save")
		if err != nil {
			utils.Error(w, "unable to create tmpdir", http.StatusInternalServerError, errors.Wrap(err, "unable to create tempdir"))
			return
		}
		output = tmpdir
	default:
		utils.Error(w, "unknown format", http.StatusInternalServerError, errors.Errorf("unknown format %q", query.Format))
		return
	}
	name := utils.GetName(r)
	newImage, err := runtime.ImageRuntime().NewFromLocal(name)
	if err != nil {
		utils.ImageNotFound(w, name, err)
		return
	}

	if err := newImage.Save(r.Context(), name, query.Format, output, []string{}, false, query.Compress); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest, err)
		return
	}
	defer os.RemoveAll(output)
	// if dir format, we need to tar it
	if query.Format == "oci-dir" || query.Format == "docker-dir" {
		rdr, err := utils2.Tar(output)
		if err != nil {
			utils.InternalServerError(w, err)
			return
		}
		defer rdr.Close()
		utils.WriteResponse(w, http.StatusOK, rdr)
		return
	}
	rdr, err := os.Open(output)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "failed to read the exported tarfile"))
		return
	}
	defer rdr.Close()
	utils.WriteResponse(w, http.StatusOK, rdr)
}

func ImagesLoad(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Reference string `schema:"reference"`
	}{
		// Add defaults here once needed.
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "failed to parse parameters for %s", r.URL.String()))
		return
	}

	tmpfile, err := ioutil.TempFile("", "libpod-images-load.tar")
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to create tempfile"))
		return
	}
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()

	if _, err := io.Copy(tmpfile, r.Body); err != nil && err != io.EOF {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to write archive to temporary file"))
		return
	}

	tmpfile.Close()
	loadedImage, err := runtime.LoadImage(context.Background(), query.Reference, tmpfile.Name(), os.Stderr, "")
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to load image"))
		return
	}
	split := strings.Split(loadedImage, ",")
	newImage, err := runtime.ImageRuntime().NewFromLocal(split[0])
	if err != nil {
		utils.InternalServerError(w, err)
		return
	}
	// TODO this should go into libpod proper at some point.
	if len(query.Reference) > 0 {
		if err := newImage.TagImage(query.Reference); err != nil {
			utils.InternalServerError(w, err)
			return
		}
	}
	utils.WriteResponse(w, http.StatusOK, entities.ImageLoadReport{Name: loadedImage})
}

func ImagesImport(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Changes   []string `schema:"changes"`
		Message   string   `schema:"message"`
		Reference string   `schema:"reference"`
		URL       string   `schema:"URL"`
	}{
		// Add defaults here once needed.
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "failed to parse parameters for %s", r.URL.String()))
		return
	}

	// Check if we need to load the image from a URL or from the request's body.
	source := query.URL
	if len(query.URL) == 0 {
		tmpfile, err := ioutil.TempFile("", "libpod-images-import.tar")
		if err != nil {
			utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to create tempfile"))
			return
		}
		defer os.Remove(tmpfile.Name())
		defer tmpfile.Close()

		if _, err := io.Copy(tmpfile, r.Body); err != nil && err != io.EOF {
			utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to write archive to temporary file"))
			return
		}

		tmpfile.Close()
		source = tmpfile.Name()
	}
	importedImage, err := runtime.Import(context.Background(), source, query.Reference, query.Changes, query.Message, true)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to import image"))
		return
	}

	utils.WriteResponse(w, http.StatusOK, entities.ImageImportReport{Id: importedImage})
}

// ImagesPull is the v2 libpod endpoint for pulling images.  Note that the
// mandatory `reference` must be a reference to a registry (i.e., of docker
// transport or be normalized to one).  Other transports are rejected as they
// do not make sense in a remote context.
func ImagesPull(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Reference    string `schema:"reference"`
		Credentials  string `schema:"credentials"`
		OverrideOS   string `schema:"overrideOS"`
		OverrideArch string `schema:"overrideArch"`
		TLSVerify    bool   `schema:"tlsVerify"`
		AllTags      bool   `schema:"allTags"`
	}{
		TLSVerify: true,
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "failed to parse parameters for %s", r.URL.String()))
		return
	}

	if len(query.Reference) == 0 {
		utils.InternalServerError(w, errors.New("reference parameter cannot be empty"))
		return
	}

	imageRef, err := utils.ParseDockerReference(query.Reference)
	if err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "image destination %q is not a docker-transport reference", query.Reference))
		return
	}

	// Trim the docker-transport prefix.
	rawImage := strings.TrimPrefix(query.Reference, fmt.Sprintf("%s://", docker.Transport.Name()))

	// all-tags doesn't work with a tagged reference, so let's check early
	namedRef, err := reference.Parse(rawImage)
	if err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "error parsing reference %q", rawImage))
		return
	}
	if _, isTagged := namedRef.(reference.Tagged); isTagged && query.AllTags {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Errorf("reference %q must not have a tag for all-tags", rawImage))
		return
	}

	var registryCreds *types.DockerAuthConfig
	if len(query.Credentials) != 0 {
		creds, err := util.ParseRegistryCreds(query.Credentials)
		if err != nil {
			utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
				errors.Wrapf(err, "error parsing credentials %q", query.Credentials))
			return
		}
		registryCreds = creds
	}

	// Setup the registry options
	dockerRegistryOptions := image.DockerRegistryOptions{
		DockerRegistryCreds: registryCreds,
		OSChoice:            query.OverrideOS,
		ArchitectureChoice:  query.OverrideArch,
	}
	if _, found := r.URL.Query()["tlsVerify"]; found {
		dockerRegistryOptions.DockerInsecureSkipTLSVerify = types.NewOptionalBool(!query.TLSVerify)
	}

	// Prepare the images we want to pull
	imagesToPull := []string{}
	res := []handlers.LibpodImagesPullReport{}
	imageName := namedRef.String()

	if !query.AllTags {
		imagesToPull = append(imagesToPull, imageName)
	} else {
		systemContext := image.GetSystemContext("", "", false)
		tags, err := docker.GetRepositoryTags(context.Background(), systemContext, imageRef)
		if err != nil {
			utils.InternalServerError(w, errors.Wrap(err, "error getting repository tags"))
			return
		}
		for _, tag := range tags {
			imagesToPull = append(imagesToPull, fmt.Sprintf("%s:%s", imageName, tag))
		}
	}

	authfile := ""
	if sys := runtime.SystemContext(); sys != nil {
		dockerRegistryOptions.DockerCertPath = sys.DockerCertPath
		authfile = sys.AuthFilePath
	}

	// Finally pull the images
	for _, img := range imagesToPull {
		newImage, err := runtime.ImageRuntime().New(
			context.Background(),
			img,
			"",
			authfile,
			os.Stderr,
			&dockerRegistryOptions,
			image.SigningOptions{},
			nil,
			util.PullImageAlways)
		if err != nil {
			utils.InternalServerError(w, errors.Wrapf(err, "error pulling image %q", query.Reference))
			return
		}
		res = append(res, handlers.LibpodImagesPullReport{ID: newImage.ID()})
	}

	utils.WriteResponse(w, http.StatusOK, res)
}

// PushImage is the handler for the compat http endpoint for pushing images.
func PushImage(w http.ResponseWriter, r *http.Request) {
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	runtime := r.Context().Value("runtime").(*libpod.Runtime)

	query := struct {
		Credentials string `schema:"credentials"`
		Destination string `schema:"destination"`
		TLSVerify   bool   `schema:"tlsVerify"`
	}{
		// This is where you can override the golang default value for one of fields
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, "Something went wrong.", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}

	source := strings.TrimSuffix(utils.GetName(r), "/push") // GetName returns the entire path
	if _, err := utils.ParseStorageReference(source); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "image source %q is not a containers-storage-transport reference", source))
		return
	}

	destination := query.Destination
	if destination == "" {
		destination = source
	}

	if _, err := utils.ParseDockerReference(destination); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "image destination %q is not a docker-transport reference", destination))
		return
	}

	newImage, err := runtime.ImageRuntime().NewFromLocal(source)
	if err != nil {
		utils.ImageNotFound(w, source, errors.Wrapf(err, "Failed to find image %s", source))
		return
	}

	var registryCreds *types.DockerAuthConfig
	if len(query.Credentials) != 0 {
		creds, err := util.ParseRegistryCreds(query.Credentials)
		if err != nil {
			utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
				errors.Wrapf(err, "error parsing credentials %q", query.Credentials))
			return
		}
		registryCreds = creds
	}

	// TODO: the X-Registry-Auth header is not checked yet here nor in any other
	// endpoint. Pushing does NOT work with authentication at the moment.
	dockerRegistryOptions := &image.DockerRegistryOptions{
		DockerRegistryCreds: registryCreds,
	}
	authfile := ""
	if sys := runtime.SystemContext(); sys != nil {
		dockerRegistryOptions.DockerCertPath = sys.DockerCertPath
		authfile = sys.AuthFilePath
	}
	if _, found := r.URL.Query()["tlsVerify"]; found {
		dockerRegistryOptions.DockerInsecureSkipTLSVerify = types.NewOptionalBool(!query.TLSVerify)
	}

	err = newImage.PushImageToHeuristicDestination(
		context.Background(),
		destination,
		"", // manifest type
		authfile,
		"", // digest file
		"", // signature policy
		os.Stderr,
		false, // force compression
		image.SigningOptions{},
		dockerRegistryOptions,
		nil, // additional tags
	)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusBadRequest, errors.Wrapf(err, "Error pushing image %q", destination))
		return
	}

	utils.WriteResponse(w, http.StatusOK, "")
}

func CommitContainer(w http.ResponseWriter, r *http.Request) {
	var (
		destImage string
		mimeType  string
	)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	runtime := r.Context().Value("runtime").(*libpod.Runtime)

	query := struct {
		Author    string   `schema:"author"`
		Changes   []string `schema:"changes"`
		Comment   string   `schema:"comment"`
		Container string   `schema:"container"`
		Format    string   `schema:"format"`
		Pause     bool     `schema:"pause"`
		Repo      string   `schema:"repo"`
		Tag       string   `schema:"tag"`
	}{
		Format: "oci",
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, "Something went wrong.", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}
	rtc, err := runtime.GetConfig()
	if err != nil {
		utils.Error(w, "failed to get runtime config", http.StatusInternalServerError, errors.Wrap(err, "failed to get runtime config"))
		return
	}
	sc := image2.GetSystemContext(rtc.Engine.SignaturePolicyPath, "", false)
	tag := "latest"
	options := libpod.ContainerCommitOptions{
		Pause: true,
	}
	switch query.Format {
	case "oci":
		mimeType = buildah.OCIv1ImageManifest
		if len(query.Comment) > 0 {
			utils.InternalServerError(w, errors.New("messages are only compatible with the docker image format (-f docker)"))
			return
		}
	case "docker":
		mimeType = manifest.DockerV2Schema2MediaType
	default:
		utils.InternalServerError(w, errors.Errorf("unrecognized image format %q", query.Format))
		return
	}
	options.CommitOptions = buildah.CommitOptions{
		SignaturePolicyPath:   rtc.Engine.SignaturePolicyPath,
		ReportWriter:          os.Stderr,
		SystemContext:         sc,
		PreferredManifestType: mimeType,
	}

	if len(query.Tag) > 0 {
		tag = query.Tag
	}
	options.Message = query.Comment
	options.Author = query.Author
	options.Pause = query.Pause
	options.Changes = query.Changes
	ctr, err := runtime.LookupContainer(query.Container)
	if err != nil {
		utils.Error(w, "failed to lookup container", http.StatusNotFound, err)
		return
	}

	// I know mitr hates this ... but doing for now
	if len(query.Repo) > 1 {
		destImage = fmt.Sprintf("%s:%s", query.Repo, tag)
	}

	commitImage, err := ctr.Commit(r.Context(), destImage, options)
	if err != nil && !strings.Contains(err.Error(), "is not running") {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrapf(err, "CommitFailure"))
		return
	}
	utils.WriteResponse(w, http.StatusOK, handlers.IDResponse{ID: commitImage.ID()}) // nolint
}

func UntagImage(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)

	name := utils.GetName(r)
	newImage, err := runtime.ImageRuntime().NewFromLocal(name)
	if err != nil {
		utils.ImageNotFound(w, name, errors.Wrapf(err, "Failed to find image %s", name))
		return
	}
	tag := "latest"
	if len(r.Form.Get("tag")) > 0 {
		tag = r.Form.Get("tag")
	}
	if len(r.Form.Get("repo")) < 1 {
		utils.Error(w, "repo tag is required", http.StatusBadRequest, errors.New("repo parameter is required to tag an image"))
		return
	}
	repo := r.Form.Get("repo")
	tagName := fmt.Sprintf("%s:%s", repo, tag)
	if err := newImage.UntagImage(tagName); err != nil {
		utils.Error(w, "failed to untag", http.StatusInternalServerError, err)
		return
	}
	utils.WriteResponse(w, http.StatusCreated, "")
}
