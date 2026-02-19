import{j as a}from"./jsx-runtime-D_zvdyIk.js";import{a as f,h as g,d as h,H as S}from"./handlers-Nn_8ejmi.js";import{S as _}from"./ServiceConfigViewer-aCfq1x7V.js";import"./iframe-CIsN2Fyl.js";import"./preload-helper-Dp1pzeXC.js";import"./fixtures-CTg5x2hB.js";const{fn:w}=__STORYBOOK_MODULE_TEST__,H={title:"Components/ServiceConfigViewer",component:_,decorators:[u=>a.jsx("div",{className:"h-[600px] bg-bg p-8",children:a.jsx(u,{})})],parameters:{msw:{handlers:f}},args:{onToast:w()}},e={},r={},s={parameters:{msw:{handlers:[g.get("/api/config/files",async()=>(await h(100),S.json([])))]}}};var t,o,n;e.parameters={...e.parameters,docs:{...(t=e.parameters)==null?void 0:t.docs,source:{originalSource:"{}",...(n=(o=e.parameters)==null?void 0:o.docs)==null?void 0:n.source}}};var i,p,c;r.parameters={...r.parameters,docs:{...(i=r.parameters)==null?void 0:i.docs,source:{originalSource:"{}",...(c=(p=r.parameters)==null?void 0:p.docs)==null?void 0:c.source}}};var m,d,l;s.parameters={...s.parameters,docs:{...(m=s.parameters)==null?void 0:m.docs,source:{originalSource:`{
  parameters: {
    msw: {
      handlers: [http.get('/api/config/files', async () => {
        await delay(100);
        return HttpResponse.json([]);
      })]
    }
  }
}`,...(l=(d=s.parameters)==null?void 0:d.docs)==null?void 0:l.source}}};const R=["Default","WithOverride","EmptyFileList"];export{e as Default,s as EmptyFileList,r as WithOverride,R as __namedExportsOrder,H as default};
