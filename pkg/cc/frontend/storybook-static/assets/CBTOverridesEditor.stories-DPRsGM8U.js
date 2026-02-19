import{j as m}from"./jsx-runtime-D_zvdyIk.js";import{c as h,h as o,d as t,H as i}from"./handlers-Nn_8ejmi.js";import{C as b}from"./CBTOverridesEditor-D3pWdD-H.js";import{m as n}from"./fixtures-CTg5x2hB.js";import"./iframe-CIsN2Fyl.js";import"./preload-helper-Dp1pzeXC.js";import"./Spinner-CVL8Mf24.js";const{fn:O}=__STORYBOOK_MODULE_TEST__,B={title:"Components/CBTOverridesEditor",component:b,decorators:[e=>m.jsx("div",{className:"bg-bg p-8",children:m.jsx(e,{})})],parameters:{msw:{handlers:h}},args:{onToast:O()}},r={},a={parameters:{msw:{handlers:[o.get("/api/config/overrides",async()=>(await t(150),i.json({...n,transformationModels:n.transformationModels.map(e=>({...e,enabled:!0})),externalModels:n.externalModels.map(e=>({...e,enabled:e.name==="beacon_api_eth_v1_beacon_block"}))}))),...h.slice(1)]}}},s={parameters:{msw:{handlers:[o.get("/api/config/overrides",async()=>(await t(999999),i.json(n))),o.get("/api/config",async()=>(await t(999999),i.json({mode:"local"})))]}}};var c,d,p;r.parameters={...r.parameters,docs:{...(c=r.parameters)==null?void 0:c.docs,source:{originalSource:"{}",...(p=(d=r.parameters)==null?void 0:d.docs)==null?void 0:p.source}}};var l,g,u;a.parameters={...a.parameters,docs:{...(l=a.parameters)==null?void 0:l.docs,source:{originalSource:`{
  parameters: {
    msw: {
      handlers: [http.get('/api/config/overrides', async () => {
        await delay(150);
        return HttpResponse.json({
          ...mockCBTOverrides,
          transformationModels: mockCBTOverrides.transformationModels.map(m => ({
            ...m,
            enabled: true
          })),
          externalModels: mockCBTOverrides.externalModels.map(m => ({
            ...m,
            enabled: m.name === 'beacon_api_eth_v1_beacon_block'
          }))
        });
      }), ...cbtOverridesHandlers.slice(1)]
    }
  }
}`,...(u=(g=a.parameters)==null?void 0:g.docs)==null?void 0:u.source}}};var f,_,v;s.parameters={...s.parameters,docs:{...(f=s.parameters)==null?void 0:f.docs,source:{originalSource:`{
  parameters: {
    msw: {
      handlers: [http.get('/api/config/overrides', async () => {
        await delay(999999);
        return HttpResponse.json(mockCBTOverrides);
      }), http.get('/api/config', async () => {
        await delay(999999);
        return HttpResponse.json({
          mode: 'local'
        });
      })]
    }
  }
}`,...(v=(_=s.parameters)==null?void 0:_.docs)==null?void 0:v.source}}};const k=["Default","WithMissingDeps","Loading"];export{r as Default,s as Loading,a as WithMissingDeps,k as __namedExportsOrder,B as default};
