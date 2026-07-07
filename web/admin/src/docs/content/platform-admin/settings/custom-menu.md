# Custom Menu Items

You can add your own links to the client portal navigation — a support desk, a status page, external monitoring, documentation, whatever fits. Custom menu items land in a dedicated section of the client portal sidebar and open the URL you point them at.

## Where to set them up

Go to **System > Custom menu** in the admin portal and choose **Add menu item**.

![Add menu item form](/docs-img/custom-menu-item-form.png)

## The fields

| Field | Description | Example |
|---|---|---|
| Display name | The label shown in the client sidebar. | `Support` |
| URL | The link target, usually an external tool. | `https://support.example.com` |
| Icon | A Font Awesome 5 (free) icon class rendered beside the label. | `fas fa-headset` |

Browse the available icons in the Font Awesome icon gallery; any class from the free set works.

## How it behaves

- Hit **Save** and the item goes live immediately — clients see it on their next page load, with no redeploy.
- Items are listed together in the custom section of the client portal navigation.

![Custom menu link in the client sidebar](/docs-img/custom-menu-item-client-sidebar.png)
