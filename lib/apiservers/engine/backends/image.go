// Copyright 2016-2017 VMware, Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package backends

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"golang.org/x/net/context"

	"github.com/docker/docker/pkg/streamformatter"
	"github.com/docker/docker/reference"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/filters"
	"github.com/docker/engine-api/types/registry"

	"github.com/vmware/vic/lib/apiservers/engine/backends/cache"
	vicfilter "github.com/vmware/vic/lib/apiservers/engine/backends/filter"
	"github.com/vmware/vic/lib/apiservers/portlayer/client/storage"
	"github.com/vmware/vic/lib/imagec"
	"github.com/vmware/vic/lib/metadata"
	"github.com/vmware/vic/lib/portlayer/util"
	"github.com/vmware/vic/pkg/trace"
	"github.com/vmware/vic/pkg/uid"
	"github.com/vmware/vic/pkg/vsphere/sys"
)

// valid filters as of docker commit 49bf474
var acceptedImageFilterTags = map[string]bool{
	"dangling":  true,
	"label":     true,
	"before":    true,
	"since":     true,
	"reference": true,
}

// currently not supported by vic
var unSupportedImageFilters = map[string]bool{
	"dangling": false,
}

type Image struct {
}

func (i *Image) Commit(name string, config *types.ContainerCommitConfig) (imageID string, err error) {
	return "", fmt.Errorf("%s does not implement image.Commit", ProductName())
}

func (i *Image) Exists(containerName string) bool {
	return false
}

// TODO fix the errors so the client doesnt print the generic POST or DELETE message
func (i *Image) ImageDelete(imageRef string, force, prune bool) ([]types.ImageDelete, error) {
	defer trace.End(trace.Begin(imageRef))

	var (
		deletedRes  []types.ImageDelete
		userRefIsID bool
	)

	// Use the image cache to go from the reference to the ID we use in the image store
	img, err := cache.ImageCache().Get(imageRef)
	if err != nil {
		return nil, err
	}

	// Get the tags from the repo cache for this image
	// TODO: remove this -- we have it in the image above
	tags := cache.RepositoryCache().Tags(img.ImageID)

	// did the user pass an id or partial id
	userRefIsID = cache.ImageCache().IsImageID(imageRef)
	// do we have any reference conflicts
	if len(tags) > 1 && userRefIsID && !force {
		t := uid.Parse(img.ImageID).Truncate()
		return nil,
			fmt.Errorf("conflict: unable to delete %s (must be forced) - image is referenced in one or more repositories", t)
	}

	// if we have an ID or only 1 tag lets delete the vmdk(s) via the PL
	if userRefIsID || len(tags) == 1 {
		log.Infof("Deleting image via PL %s (%s)", img.ImageID, img.ID)

		// storeName is the uuid of the host this service is running on.
		storeName, err := sys.UUID()
		if err != nil {
			return nil, err
		}

		// We're going to delete all of the images in the layer branch starting
		// at the given leaf.  BUT!  we need to keep the images which may be
		// referenced by tags.  Therefore, we need to assemble a list of images
		// (by URI) which are referred to by tags.
		allImages := cache.ImageCache().GetImages()
		keepNodes := make([]string, len(allImages))
		for idx, node := range allImages {
			imgURL, err := util.ImageURL(storeName, node.ImageID)
			if err != nil {
				return nil, err
			}

			keepNodes[idx] = imgURL.String()
		}

		params := storage.NewDeleteImageParamsWithContext(ctx).WithStoreName(storeName).WithID(img.ID).WithKeepNodes(keepNodes)
		// TODO: This will fail if any containerVMs are referencing the vmdk - vanilla docker
		// allows the removal of an image (via force flag) even if a container is referencing it
		// should vic?
		res, err := PortLayerClient().Storage.DeleteImage(params)

		// We may have deleted images despite error.  Account for that in the cache.
		if res != nil {
			for _, deletedImage := range res.Payload {

				// map the layer id to the blob sum so the ids map to what we
				// present to the user on pull
				id := deletedImage.ID
				i, err := imagec.LayerCache().Get(deletedImage.ID)
				if err == nil {
					id = i.Layer.BlobSum
				}

				// remove the layer from the layer cache (used by imagec)
				imagec.LayerCache().Remove(deletedImage.ID)

				// form the response
				imageDeleted := types.ImageDelete{Deleted: strings.TrimPrefix(id, "sha256:")}
				deletedRes = append(deletedRes, imageDeleted)
			}
		}

		if err != nil {
			switch err := err.(type) {
			case *storage.DeleteImageLocked:
				return nil, fmt.Errorf("Failed to remove image %q: %s", imageRef, err.Payload.Message)
			default:
				return nil, err
			}
		}

		// we've deleted the image so remove from cache
		cache.ImageCache().RemoveImageByConfig(img)

	} else {

		// only untag the ref supplied
		n, err := reference.ParseNamed(imageRef)
		if err != nil {
			return nil, fmt.Errorf("unable to parse reference(%s): %s", imageRef, err.Error())
		}
		tag := reference.WithDefaultTag(n)
		tags = []string{tag.String()}
	}
	// loop thru and remove from repoCache
	for i := range tags {
		// remove from cache, but don't save -- we'll do that afer all
		// updates
		refNamed, _ := cache.RepositoryCache().Remove(tags[i], false)
		deletedRes = append(deletedRes, types.ImageDelete{Untagged: refNamed})
	}

	// save repo now -- this will limit the number of PL
	// calls to one per rmi call
	err = cache.RepositoryCache().Save()
	if err != nil {
		return nil, fmt.Errorf("Untag error: %s", err.Error())
	}

	return deletedRes, err
}

func (i *Image) ImageHistory(imageName string) ([]*types.ImageHistory, error) {
	return nil, fmt.Errorf("%s does not implement image.History", ProductName())
}

func (i *Image) Images(filterArgs string, filter string, all bool) ([]*types.Image, error) {
	defer trace.End(trace.Begin(fmt.Sprintf("filterArgs: %s", filterArgs)))

	// This type conversion can be removed once we move to 1.13
	// At 1.13 the Images func will change signatures and filterArgs will be properly
	// typed
	imageFilters, err := filters.FromParam(filterArgs)
	if err != nil {
		return nil, err
	}

	// utilize the filterArgs to incorporate the argument option
	if filter != "" {
		imageFilters.Add("reference", filter)
	}

	// validate filters for accuracy and support
	filterContext, err := vicfilter.ValidateImageFilters(imageFilters, acceptedImageFilterTags, unSupportedImageFilters)
	if err != nil {
		return nil, err
	}

	// get all images
	images := cache.ImageCache().GetImages()

	result := make([]*types.Image, 0, len(images))

imageLoop:
	for i := range images {

		// provide filter with current ImageID
		filterContext.ID = images[i].ImageID

		// provide image labels
		if images[i].Config != nil {
			filterContext.Labels = images[i].Config.Labels
		}

		// determine if image should be part of list
		action := vicfilter.IncludeImage(imageFilters, filterContext)

		switch action {
		case vicfilter.ExcludeAction:
			continue imageLoop
		case vicfilter.StopAction:
			break imageLoop
		}
		// if we are here then add image
		result = append(result, convertV1ImageToDockerImage(images[i]))
	}

	return result, nil
}

// Docker Inspect.  LookupImage looks up an image by name and returns it as an
// ImageInspect structure.
func (i *Image) LookupImage(name string) (*types.ImageInspect, error) {
	defer trace.End(trace.Begin("LookupImage (docker inspect)"))

	imageConfig, err := cache.ImageCache().Get(name)
	if err != nil {
		return nil, err
	}

	return imageConfigToDockerImageInspect(imageConfig, ProductName()), nil
}

func (i *Image) TagImage(newTag reference.Named, imageName string) error {
	return fmt.Errorf("%s does not implement image.Tag", ProductName())
}

func (i *Image) LoadImage(inTar io.ReadCloser, outStream io.Writer, quiet bool) error {
	return fmt.Errorf("%s does not implement image.LoadImage", ProductName())
}

func (i *Image) ImportImage(src string, newRef reference.Named, msg string, inConfig io.ReadCloser, outStream io.Writer, config *container.Config) error {
	return fmt.Errorf("%s does not implement image.ImportImage", ProductName())
}

func (i *Image) ExportImage(names []string, outStream io.Writer) error {
	return fmt.Errorf("%s does not implement image.ExportImage", ProductName())
}

func (i *Image) PullImage(ctx context.Context, ref reference.Named, metaHeaders map[string][]string, authConfig *types.AuthConfig, outStream io.Writer) error {
	defer trace.End(trace.Begin(ref.String()))

	log.Debugf("PullImage: ref = %+v, metaheaders = %+v\n", ref, metaHeaders)

	options := imagec.Options{
		Destination: os.TempDir(),
		Reference:   ref.String(),
		Timeout:     imagec.DefaultHTTPTimeout,
		Outstream:   outStream,
		RegistryCAs: RegistryCertPool,
	}

	if authConfig != nil {
		if len(authConfig.Username) > 0 {
			options.Username = authConfig.Username
		}
		if len(authConfig.Password) > 0 {
			options.Password = authConfig.Password
		}
	}

	portLayerServer := PortLayerServer()

	if portLayerServer != "" {
		options.Host = portLayerServer
	}

	insecureRegistries := InsecureRegistries()
	for _, registry := range insecureRegistries {
		if registry == ref.Hostname() {
			options.InsecureAllowHTTP = true
			break
		}
	}

	log.Infof("PullImage: reference: %s, %s, portlayer: %#v",
		options.Reference,
		options.Host,
		portLayerServer)

	ic := imagec.NewImageC(options, streamformatter.NewJSONStreamFormatter())
	err := ic.PullImage()
	if err != nil {
		return err
	}

	return nil
}

func (i *Image) PushImage(ctx context.Context, ref reference.Named, metaHeaders map[string][]string, authConfig *types.AuthConfig, outStream io.Writer) error {
	return fmt.Errorf("%s does not implement image.PushImage", ProductName())
}

func (i *Image) SearchRegistryForImages(ctx context.Context, term string, authConfig *types.AuthConfig, metaHeaders map[string][]string) (*registry.SearchResults, error) {
	return nil, fmt.Errorf("%s does not implement image.SearchRegistryForImages", ProductName())
}

// Utility functions

func convertV1ImageToDockerImage(image *metadata.ImageConfig) *types.Image {
	var labels map[string]string
	if image.Config != nil {
		labels = image.Config.Labels
	}

	return &types.Image{
		ID:          image.ImageID,
		ParentID:    image.Parent,
		RepoTags:    image.Tags,
		RepoDigests: image.Digests,
		Created:     image.Created.Unix(),
		Size:        image.Size,
		VirtualSize: image.Size,
		Labels:      labels,
	}
}

// Converts the data structure retrieved from the portlayer.  This src datastructure
// represents the unmarshalled data saved in the storage port layer.  The return
// data is what the Docker CLI understand and returns to user.
func imageConfigToDockerImageInspect(imageConfig *metadata.ImageConfig, productName string) *types.ImageInspect {
	if imageConfig == nil {
		return nil
	}

	rootfs := types.RootFS{
		Type:      "layers",
		Layers:    make([]string, 0, len(imageConfig.History)),
		BaseLayer: "",
	}

	for k := range imageConfig.DiffIDs {
		rootfs.Layers = append(rootfs.Layers, k)
	}

	inspectData := &types.ImageInspect{
		RepoTags:        imageConfig.Tags,
		RepoDigests:     imageConfig.Digests,
		Parent:          imageConfig.Parent,
		Comment:         imageConfig.Comment,
		Created:         imageConfig.Created.Format(time.RFC3339Nano),
		Container:       imageConfig.Container,
		ContainerConfig: &imageConfig.ContainerConfig,
		DockerVersion:   imageConfig.DockerVersion,
		Author:          imageConfig.Author,
		Config:          imageConfig.Config,
		Architecture:    imageConfig.Architecture,
		Os:              imageConfig.OS,
		Size:            imageConfig.Size,
		VirtualSize:     imageConfig.Size,
		RootFS:          rootfs,
	}

	inspectData.GraphDriver.Name = productName + " " + PortlayerName

	//imageid is currently stored within VIC without "sha256:" so we add it to
	//match Docker
	inspectData.ID = "sha256:" + imageConfig.ImageID

	return inspectData
}
